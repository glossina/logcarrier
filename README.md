# logcarrier
Logfile tailing/delivery system. Initially forked from https://github.com/Boiler/logcarrier with a strong intention to completely rewrite it. Done.

Config format:

```yaml
listen: 0.0.0.0:1146
listen_debug: 0.0.0.0:40000      # This port can be connected to check service availability
wait_timeout: 1m13s
key: '123123123123'
logfile:                         # stderr will be used if this parameter is not set

compression:
  method: zstd                   # Can be `zstd` or `raw` for no compression
  level: 6

buffers:  
  # Buffer order is: tailer -> input buffer -> [compressor ->] frame buffer -> disk
  
  input: 128Kb                   # Kb, Mb, Gb can be used (or just number in bytes). This is input buffer
                                 # that guranties line integrity
  framing: 256Kb                 # Same format. this buffer ensures frame integrity which is critically important
                                 # for compressed output: broken frame will cause decompressing errors
  zstdict: 128Kb                 # ZSTD compression dictionary size. Probably a good thing
  connections: 1024              # How many connections to allow at the moment
  dumps: 512                     # Dumping file is a task too. How many dumping tasks to can be set without a service denial.
  logrotates: 512                # Same as previous, just for log rotation

workers:
  route: 1024                    # How many workers process incoming connections
  dumper: 24                     # How many workers process dumping data
  logrotater: 12                 # How many log rotation workers

  flusher_sleep: 30s             # Intervals for force flush

files:
  root: /var/logs/logcarrier                  # Root directory
  root_mode: 0755                             # Mode for subdirectories creating in a process
  name: /$dir/$name-${time | %Y%m%d%H }       # File name template
  rotation: /$dir/$name-${ time | %Y%m%d%H }  # Rename to on rotation. This time the same name.

links:                           # Same as with files
  root: ..
  root_mode: ..
  name: ..
  rotation: ..

logrotate:
  method: periodic              # Can be `periodic`, `guided` (via protocol) and `both`
  schedule: "* */1 * * *"       # Log rotation start schedule
```
