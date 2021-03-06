resque_exporter
==

An exporter of [Prometheus](https://prometheus.io) for [resque](https://github.com/resque/resque)'s status.

Usage
--

```
Options:

  -h, --help            display help
  -v, --version         display version and revision
  -p, --port[=9102]     set port number
```

e.g.

```
$ REDIS_HOST=localhost REDIS_PORT=6379 RESQUE_NAMESPACE=resque GUARD_INTERVAL_MILLIS=0 ./resque_exporter
```

Description
--

This exporter exports following items.

- Number of remained jobs in queue
- Number of processed jobs
- Number of failed jobs
- Number of jobs in the `failed` queue
- Number of total workers
- Number of active workers
- Number of idle workers

Paths that supported by this exporter
--

| Paths    | Description           |
| -------- | --------------------- |
| /metrics | Exports metrics       |
| /\*      | Path for health check |

Configuration
--

Configuration is managed through the following environment variables:

| Variable              | Default               |
| --------------------- | --------------------- |
| REDIS_HOST            | **Required**          |
| REDIS_PORT            | 6379                  |
| RESQUE_NAMESPACE      | resque                |
| GUARD_INTERVAL_MILLIS | 0                     |


Sample Output
--

```
# HELP resque_jobs_in_queue Number of remained jobs in queue
# TYPE resque_jobs_in_queue gauge
resque_jobs_in_queue{queue_name="image_converting"} 0
resque_jobs_in_queue{queue_name="log_compression"} 0
# HELP resque_failed Number of failed jobs
# TYPE resque_failed gauge
resque_failed 123
# HELP failed_queue_count Number of jobs in the failed queue
# TYPE failed_queue_count gauge
failed_queue_count 1
# HELP resque_processed Number of processed jobs
# TYPE resque_processed gauge
resque_processed 1.234567e+06
# HELP resque_active_workers Number of active workers
# TYPE resque_active_workers gauge
resque_active_workers 2
# HELP resque_idle_workers Number of idle workers
# TYPE resque_idle_workers gauge
resque_idle_workers 8
# HELP resque_total_workers Number of workers
# TYPE resque_total_workers gauge
resque_total_workers 10
```

Mechanism
--

This exporter accesses to redis to aggregate status.

1. Collect name of queues via `<namespace>:queues` entry (by using [SMEMBERS](http://redis.io/commands/smembers))
1. Get number of remained jobs for each queue via `<namespace>:queue:<queue_name>` entry (by using [LLEN](http://redis.io/commands/llen))
1. Get number of processed jobs via `<namespace>:stat:processed`
1. Get number of failed jobs via `<namespace>:stat:failed`
1. Collect name of workers via `<namespace>:workers` entry (by using [SMEMBERS](http://redis.io/commands/smembers))
1. Count number of active workers and idle workers by getting for each workers status entry `<namespace>:worker:<worker_name>`.

Health Check
--

Any paths that except for `/metrics` returns response for health check. It returns 200 HTTP status code.

Note
--

This exporter also supports resque compatible job-queue engines (e.g. [jesque](https://github.com/gresrun/jesque)).

License
--

```
The MIT License (MIT)
Copyright © 2018 Barefoot Coders, http://www.barefootcoders.com/ <jason@barefootcoders.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the “Software”), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
```

