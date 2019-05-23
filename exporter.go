package resqueExporter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/redis.v3"
)

const namespace = "resque"

type exporter struct {
	config              *Config
	mut                 sync.Mutex
	scrapeFailures      prometheus.Counter
	processed           prometheus.Gauge
	failedQueue         prometheus.Gauge
	failedTotal         prometheus.Gauge
	queueStatus         *prometheus.GaugeVec
	failuresByQueueName *prometheus.GaugeVec
	failuresByException *prometheus.GaugeVec
	totalWorkers        prometheus.Gauge
	activeWorkers       prometheus.Gauge
	idleWorkers         prometheus.Gauge
	dirtyExits          *prometheus.GaugeVec
	sigkilledWorkers    *prometheus.GaugeVec
	timer               *time.Timer
}

type Payload struct {
	Class string        `json:"class"`
	Args  []interface{} `json:"args"`
}

type failure struct {
	// FailedAt  time.Time `json:"failed_at"`
	Payload   Payload  `json:"payload"`
	Exception string   `json:"exception"`
	Error     string   `json:"error"`
	Backtrace []string `json:"backtrace"`
	Worker    string   `json:"worker"`
	Queue     string   `json:"queue"`
}

func newExporter(config *Config) (*exporter, error) {
	e := &exporter{
		config: config,
		queueStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "jobs_in_queue",
				Help:      "Number of remained jobs in queue",
			},
			[]string{"namespace", "queue_name", "worker_name", "deployment"},
		),
		failuresByQueueName: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "failures_by_queue_name",
				Help:      "Failures by queue name",
			},
			[]string{"queue_name"},
		),
		failuresByException: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "failures_by_exception",
				Help:      "Failures by exception",
			},
			[]string{"exception"},
		),
		processed: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "processed",
			Help:      "Number of processed jobs",
		}),
		failedQueue: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "failed_queue_count",
			Help:      "Number of jobs in the failed queue",
		}),
		failedTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "failed",
			Help:      "Number of failed jobs",
		}),
		scrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_scrape_failures_total",
			Help:      "Number of errors while scraping resque.",
		}),
		totalWorkers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "total_workers",
			Help:      "Number of workers",
		}),
		activeWorkers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_workers",
			Help:      "Number of active workers",
		}),
		idleWorkers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "idle_workers",
			Help:      "Number of idle workers",
		}),
		dirtyExits: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "dirty_exited_jobs",
				Help:      "Number of Resque jobs subject to a DirtyExit",
			},
			[]string{"queue_name"},
		),
		sigkilledWorkers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "sigkilled_workers",
				Help:      "Number of failed Resque jobs that were SIGKILLed",
			},
			[]string{"queue_name"},
		),
	}

	return e, nil
}

func (e *exporter) Describe(ch chan<- *prometheus.Desc) {
	e.scrapeFailures.Describe(ch)
	e.queueStatus.Describe(ch)
	e.failuresByQueueName.Describe(ch)
	e.failuresByException.Describe(ch)
	e.processed.Describe(ch)
	e.failedQueue.Describe(ch)
	e.failedTotal.Describe(ch)
	e.totalWorkers.Describe(ch)
	e.activeWorkers.Describe(ch)
	e.idleWorkers.Describe(ch)
	e.sigkilledWorkers.Describe(ch)
}

func (e *exporter) Collect(ch chan<- prometheus.Metric) {
	e.mut.Lock() // To protect metrics from concurrent collects.
	defer e.mut.Unlock()

	if e.timer != nil {
		// guarded
		e.notifyToCollect(ch)
		return
	}

	if err := e.collect(ch); err != nil {
		e.incrementFailures(ch)
	}

	e.timer = time.AfterFunc(time.Duration(e.config.GuardIntervalMillis)*time.Millisecond, func() {
		// reset timer
		e.mut.Lock()
		defer e.mut.Unlock()
		e.timer = nil
	})
}

func (e *exporter) collect(ch chan<- prometheus.Metric) error {
	resqueNamespace := e.config.ResqueNamespace

	redisConfig := e.config.Redis
	redisOpt := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", redisConfig.Host, redisConfig.Port),
		Password: redisConfig.Password,
		DB:       redisConfig.DB,
	}
	redis := redis.NewClient(redisOpt)
	defer redis.Close()

	failed, err := redis.LLen(fmt.Sprintf("%s:failed", resqueNamespace)).Result()
	if err != nil {
		return err
	}
	e.failedQueue.Set(float64(failed))

	if e.config.ExportFailureDetails {
		failures, err := redis.LRange(fmt.Sprintf("%s:failed", resqueNamespace), 0, failed).Result()
		if err != nil {
			return err
		}

		failuresByQueueName := make(map[string]int)
		failuresByException := make(map[string]int)
		dirtyExits := make(map[string]int)
		sigKills := make(map[string]int)

		for _, j := range failures {
			job := &failure{}
			json.Unmarshal([]byte(j), &job)
			if err != nil {
				return err
			}
			if job.Queue == "" {
				fmt.Println("==================")
				fmt.Println("WTF? Got an empty queue")
				fmt.Println(job)
				fmt.Println(j)
				fmt.Println("=== Dumping Struct ===")
				// fmt.Printf("FailedAt = %s\n", job.FailedAt)
				fmt.Printf("Payload = %s\n", job.Payload)
				/*
					if job.Payload != nil {
						fmt.Printf("=> Class = %s", job.Payload.Class)
						fmt.Printf("=> Args = %s", job.Payload.Args)
					}
				*/
				fmt.Printf("Exception = %s\n", job.Exception)
				fmt.Printf("Error = %s\n", job.Error)
				fmt.Printf("Backtrace = %s\n", job.Backtrace)
				fmt.Printf("Worker = %s\n", job.Worker)
				fmt.Printf("Queue = %s\n", job.Queue)
				fmt.Println("=== END STRUCT ===")
				fmt.Println("==================")
			}

			failuresByQueueName[job.Queue]++
			failuresByException[job.Exception]++

			if job.Exception == "Resque::DirtyExit" {
				dirtyExits[job.Queue]++
				match, err := regexp.MatchString(".*SIGKILL.*", job.Error)
				if err != nil {
					return err
				}
				if match {
					sigKills[job.Queue]++
				}
			}
		}

		for queue, count := range failuresByQueueName {
			e.failuresByQueueName.WithLabelValues(queue).Set(float64(count))
		}
		for exception, count := range failuresByException {
			e.failuresByException.WithLabelValues(exception).Set(float64(count))
		}

		for queue, count := range dirtyExits {
			e.dirtyExits.WithLabelValues(queue).Set(float64(count))
		}
		for queue, count := range sigKills {
			e.sigkilledWorkers.WithLabelValues(queue).Set(float64(count))
		}
	}

	queues, err := redis.SMembers(fmt.Sprintf("%s:queues", resqueNamespace)).Result()
	if err != nil {
		return err
	}

	for _, q := range queues {
		n, err := redis.LLen(fmt.Sprintf("%s:queue:%s", resqueNamespace, q)).Result()
		if err != nil {
			return err
		}
		workersForQueue := e.config.QueueConfiguration.QueueToWorkers[q]
		for _, worker := range workersForQueue {
			e.queueStatus.WithLabelValues("default", q, worker, worker).Set(float64(n))
		}
	}

	processed, err := redis.Get(fmt.Sprintf("%s:stat:processed", resqueNamespace)).Result()
	if err != nil {
		return err
	}
	processedCnt, _ := strconv.ParseFloat(processed, 64)
	e.processed.Set(processedCnt)

	failedtotal, err := redis.Get(fmt.Sprintf("%s:stat:failed", resqueNamespace)).Result()
	if err != nil {
		return err
	}
	failedCnt, _ := strconv.ParseFloat(failedtotal, 64)
	e.failedTotal.Set(failedCnt)

	workers, err := redis.SMembers(fmt.Sprintf("%s:workers", resqueNamespace)).Result()
	if err != nil {
		return err
	}
	e.totalWorkers.Set(float64(len(workers)))

	activeWorkers := 0
	idleWorkers := 0
	for _, w := range workers {
		_, err := redis.Get(fmt.Sprintf("%s:worker:%s", resqueNamespace, w)).Result()
		if err == nil {
			activeWorkers++
		} else {
			idleWorkers++
		}
	}
	e.activeWorkers.Set(float64(activeWorkers))
	e.idleWorkers.Set(float64(idleWorkers))

	e.notifyToCollect(ch)

	return nil
}

func (e *exporter) incrementFailures(ch chan<- prometheus.Metric) {
	e.scrapeFailures.Inc()
	e.scrapeFailures.Collect(ch)
}

func (e *exporter) notifyToCollect(ch chan<- prometheus.Metric) {
	e.queueStatus.Collect(ch)
	e.failuresByQueueName.Collect(ch)
	e.failuresByException.Collect(ch)
	e.processed.Collect(ch)
	e.failedQueue.Collect(ch)
	e.failedTotal.Collect(ch)
	e.totalWorkers.Collect(ch)
	e.activeWorkers.Collect(ch)
	e.idleWorkers.Collect(ch)
	e.dirtyExits.Collect(ch)
	e.sigkilledWorkers.Collect(ch)
}
