# Runner

Easily create distributed work-queues and submit tasks across cores, machines and architectures.

I came up with this at ~4AM. Expect a name change
---

## Dependencies

  - Redis

Host a Redis server somewhere and point Runner to it.

## Usage

### Workers

First, we need some workers. Compile the `cmd/server` program and run it on different machines to add them to the pool.

```
usage: server --work-queue=WORK-QUEUE [<flags>]

Flags:
      --help                   Show context-sensitive help (also try --help-long and --help-man).
  -v, --verbose                Enable verbose logs
  -w, --work-queue=WORK-QUEUE  Use a specific work-queue
  -j, --num-procs=12           Number of processes to run in parallel
  -R, --redis-addr="127.0.0.1:6379"  
                               Redis address
```

Each server instance can internally create multiple workers using the `-j` flag.


Once we have the workers set up, we need to submit jobs. Currently, you can do that using the `cmd/runctl` command:

### Jobs

```
usage: runctl submit [<flags>] <command>

Submit a job

Flags:
      --help                   Show context-sensitive help (also try --help-long and --help-man).
  -v, --verbose                Enable verbose logs
  -w, --work-queue=WORK-QUEUE  Use a specific work-queue
  -R, --redis-addr="127.0.0.1:6379"  
                               Redis address
  -e, --env=""                 Environment variables to set. Expected to be of the form KEY=VAR. Multiple values are separated by semi-colon
      --cwd=""                 Current working directory for the command
  -W, --wait                   Wait for result

Args:
  <command>  command-line
```

#### Examples

**Simple**
`runctl -w test -R redis.example.com submit 'ls -lah'`

**Complex**
`runctl -w test -R redis.example.com submit "/bin/bash -c 'source env; ./bin/\${ARCH}/runtests" --env LOGDIR=/nfs/logs --cwd /nfs/workspace/project --no-wait`