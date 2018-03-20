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
	failuresByError     *prometheus.GaugeVec
	failuresByWorker    *prometheus.GaugeVec
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
			[]string{"queue_name", "worker_name", "deployment"},
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
		failuresByError: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "failures_by_error",
				Help:      "Failures by error",
			},
			[]string{"error"},
		),
		failuresByWorker: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "failures_by_worker",
				Help:      "Failures by worker",
			},
			[]string{"worker"},
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

var allWorkers = []string{
	"api-resqueworker",
	"api-resqueworker-mailchimp-updater",
	"api-resqueworker-rebuild-object-cache",
	"api-resqueworker-resquebus-incoming",
	"api-resqueworker-reverse",
	"archive-resqueworker-all",
	"archive-resqueworker-external",
	"archive-resqueworker-high",
	"archive-resqueworker-instabot-sourcing",
	"archive-resqueworker-log",
	"archive-resqueworker-medium",
}

var queueToWorker = map[string][]string{
	"resquebus_incoming":                   []string{"api-resqueworker", "api-resqueworker-reverse", "resquebus_incoming"},
	"handle_merged_accounts_in_feeds":      []string{"api-resqueworker", "api-resqueworker-reverse"},
	"update_account_briefs":                []string{"api-resqueworker", "api-resqueworker-reverse"},
	"index_accounts":                       []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_delectabase_merges":            []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_updated_wine_profile":          []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_verified_identifier":           []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_updated_producer_role":         []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_deleted_producer_role":         []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_deleted_capture":               []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_context_invalidation":          []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_new_transcription":             []string{"api-resqueworker", "api-resqueworker-reverse"},
	"verify_identifiers":                   []string{"api-resqueworker", "api-resqueworker-reverse"},
	"facebook_capture":                     []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_impossible_capture":            []string{"api-resqueworker", "api-resqueworker-reverse"},
	"handle_untranscription":               []string{"api-resqueworker", "api-resqueworker-reverse"},
	"invalidate_followers_feed_signatures": []string{"api-resqueworker", "api-resqueworker-reverse"},
	"publish_activity":                     []string{"api-resqueworker", "api-resqueworker-reverse"},
	"send_push_notifications":              []string{"api-resqueworker", "api-resqueworker-reverse"},
	"resque_cleanup":                       []string{"api-resqueworker", "api-resqueworker-reverse"},
	"build_global_recommendations":         []string{"api-resqueworker", "api-resqueworker-reverse"},
	"build_recommendations":                []string{"api-resqueworker", "api-resqueworker-reverse"},
	"new_followers_digest":                 []string{"api-resqueworker", "api-resqueworker-reverse"},
	"set_badge_numbers":                    []string{"api-resqueworker", "api-resqueworker-reverse"},
	"cache_capture_lists":                  []string{"api-resqueworker", "api-resqueworker-reverse"},
	"rebuild_object_cache":                 []string{"api-resqueworker", "api-resqueworker-reverse", "api-resqueworker-rebuild-object-cache"},
	"validate_object_caches":               []string{"api-resqueworker", "api-resqueworker-reverse"},
	"track_by_sk":                          []string{"api-resqueworker", "api-resqueworker-reverse"},
	"send_analytics_events":                []string{"api-resqueworker", "api-resqueworker-reverse"},
	"notify_keen":                          []string{"api-resqueworker", "api-resqueworker-reverse"},
	"hourly_analytics":                     []string{"api-resqueworker", "api-resqueworker-reverse"},
	"daily_analytics":                      []string{"api-resqueworker", "api-resqueworker-reverse"},
	"minutely_analytics":                   []string{"api-resqueworker", "api-resqueworker-reverse"},
	"notify_mix_panel":                     []string{"api-resqueworker", "api-resqueworker-reverse"},
	"vintank_publish":                      []string{"api-resqueworker", "api-resqueworker-reverse"},
	"account_life_cycle":                   []string{"api-resqueworker", "api-resqueworker-reverse"},
	"mandrill_emailer":                     []string{"api-resqueworker", "api-resqueworker-reverse"},
	"send_sms":                             []string{"api-resqueworker", "api-resqueworker-reverse"},
	"archive_subscriber":                   []string{"api-resqueworker", "api-resqueworker-reverse"},
	"activity_subscriber":                  []string{"api-resqueworker", "api-resqueworker-reverse"},
	"facebook_subscriber":                  []string{"api-resqueworker", "api-resqueworker-reverse"},
	"vintank_subscriber":                   []string{"api-resqueworker", "api-resqueworker-reverse"},
	"context_list_subscriber":              []string{"api-resqueworker", "api-resqueworker-reverse"},
	"delectasearch_subscriber":             []string{"api-resqueworker", "api-resqueworker-reverse"},
	"gamification":                         []string{"api-resqueworker", "api-resqueworker-reverse"},
	"gamification_backfill":                []string{"api-resqueworker", "api-resqueworker-reverse"},
	"mailchimp_updater":                    []string{"api-resqueworker-mailchimp-updater"},
	"receive_crowdflower_vintage_tag":      []string{"archive-resqueworker-all", "archive-resqueworker-high"},
	"receive_crowdflower_base_wine_match":  []string{"archive-resqueworker-all", "archive-resqueworker-high"},
	"receive_crowdflower_metadata":         []string{"archive-resqueworker-all", "archive-resqueworker-high"},
	"manage_capture_queue":                 []string{"archive-resqueworker-all", "archive-resqueworker-high"},
	"manage_image_queue":                   []string{"archive-resqueworker-all", "archive-resqueworker-high"},
	"manage_sourcing":                      []string{"archive-resqueworker-all", "archive-resqueworker-high", "archive-resqueworker-medium"},
	"log_capture_event":                    []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"handle_automatic_results":             []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"sort_new_capture":                     []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"attempt_instant_match":                []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"handle_archive_transcription":         []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"log_archivist_action":                 []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"process_archivist_actions":            []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"capture_log_status_event":             []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"hourly_transcription_stats":           []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"untranscribe_image_search":            []string{"archive-resqueworker-all", "archive-resqueworker-low"},
	"merge_image_queries":                  []string{"archive-resqueworker-all", "archive-resqueworker-low"},
	"receive_wine_dot_com_capture":         []string{"archive-resqueworker-all"},
	"create_resolutions":                   []string{"archive-resqueworker-all", "archive-resqueworker-high"},
	"assign_default_photo":                 []string{"archive-resqueworker-all", "archive-resqueworker-low"},
	"delectabase_subscriber":               []string{"archive-resqueworker-all", "archive-resqueworker-medium"},
	"vinous":                               []string{"archive-resqueworker-all", "archive-instabot-sourcing"},
	"downgrade_expired_user":               []string{"archive-resqueworker-all"},
	"sourcing":                             []string{"archive-resqueworker-instabot-sourcing"},
	"post_instagram_comments":              []string{"archive-resqueworker-instabot-sourcing"},
	"receive_instagram_scrape_complete":    []string{"archive-resqueworker-instabot-sourcing"},
	"receive_instagram_user":               []string{"archive-resqueworker-instabot-sourcing"},
	"receive_instagram_capture":            []string{"archive-resqueworker-instabot-sourcing"},
	"instabot":                             []string{"archive-resqueworker-instabot-sourcing"},
	"analyze_instagram_captures":           []string{"archive-resqueworker-instabot-sourcing"},
	"external_capture":                     []string{"archive-resqueworker-medium"},
	"repopulate_photos":                    []string{"archive-resqueworker-low"},
	"delectabrain":                         []string{"archive-resqueworker-low"},
	// TODO more workers
}

func (e *exporter) Describe(ch chan<- *prometheus.Desc) {
	e.scrapeFailures.Describe(ch)
	e.queueStatus.Describe(ch)
	e.failuresByQueueName.Describe(ch)
	e.failuresByException.Describe(ch)
	e.failuresByError.Describe(ch)
	e.failuresByWorker.Describe(ch)
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

	failures, err := redis.LRange(fmt.Sprintf("%s:failed", resqueNamespace), 0, failed).Result()
	if err != nil {
		return err
	}

	failuresByQueueName := make(map[string]int)
	failuresByException := make(map[string]int)
	failuresByError := make(map[string]int)
	failuresByWorker := make(map[string]int)
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
		failuresByError[job.Error]++
		// TODO break this up a bit
		failuresByWorker[job.Worker]++

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
	for err, count := range failuresByError {
		e.failuresByError.WithLabelValues(err).Set(float64(count))
	}
	for worker, count := range failuresByWorker {
		e.failuresByWorker.WithLabelValues(worker).Set(float64(count))
	}

	for queue, count := range dirtyExits {
		e.dirtyExits.WithLabelValues(queue).Set(float64(count))
	}
	for queue, count := range sigKills {
		e.sigkilledWorkers.WithLabelValues(queue).Set(float64(count))
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
		workersForQueue := queueToWorker[q]
		for _, worker := range workersForQueue {
			e.queueStatus.WithLabelValues(q, worker, worker).Set(float64(n))
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
	e.failuresByError.Collect(ch)
	e.failuresByWorker.Collect(ch)
	e.processed.Collect(ch)
	e.failedQueue.Collect(ch)
	e.failedTotal.Collect(ch)
	e.totalWorkers.Collect(ch)
	e.activeWorkers.Collect(ch)
	e.idleWorkers.Collect(ch)
	e.dirtyExits.Collect(ch)
	e.sigkilledWorkers.Collect(ch)
}
