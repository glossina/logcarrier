# logcarrier
Logfile tailing/delivery system. Initially forked from https://github.com/Boiler/logcarrier with a strong intention to completely rewrite it for better configurability and features.

Installation:
```
go get -u github.com/sirkon/logcarrier
```

# What has been added or changed so far
1. Configured name based on input parameters. Name after log rotation. The best practice though is to initially have time segregated name for original file name and access it via "originally" named link.
2. Link for a file with similar features (current and after log rotation names)
3. ZSTD on the fly compression (can be memory hungry).
4. Use YAML instead of TOML for config. TOML can be easier to parse but its Go library is pretty poor in error handling and TOML itself doesn't support octal numbers (to describe directory rights) and things like `128Kb` at place.

# Stability and code quality
1. The core was written in a couple of days in a hurry, thus some parts of code and design choices are questionable.
2. I quit my job I was writing this for and prefered not to contact them after that, so there were no real tests. `raw` compression method should work though, not so sure about `zstd` - previous version which was based on vanilla ZSTD library worked, current one uses tweaked library version to implement rollback functionality.

Anyway, feel free to report and contribute.

# Config format

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
  zstdict: 128Kb                 # ZSTD compression dictionary size. They say this accelerates compression speed.
  connections: 1024              # How many connections attempts to allow at the moment
  dumps: 512                     # This is the length of the queue of connections from tailers awaiting for dumping their data.
  logrotates: 512                # How many log rotating tasks to queue without a block.

workers:
  route: 1024                    # How many workers process incoming connections
  dumper: 24                     # How many workers process data from tailers.
  logrotater: 12                 # How many log rotation workers

  flusher_sleep: 30s             # Intervals for force flush

files:
  root: /var/logs/logcarrier                  # Root directory
  root_mode: 0755                             # Mode for subdirectories creating in a process
  name: /$dir/$name-${time | %Y%m%d%H }       # File name template. This is a good idea to give file an already rotated name 
                                              # (date, hour, minute, etc) and use link with "original" file name pointed at the  
                                              # currently writing part
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
