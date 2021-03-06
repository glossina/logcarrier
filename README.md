# logcarrier
Logfile tailing/delivery system. Initially forked from https://github.com/Boiler/logcarrier with a strong intention to completely rewrite it for better configurability and features.

Installation:
```
go get -u github.com/sirkon/logcarrier
```
#### What does this thing is intended to do
Carry logs from remote machines into one place

#### How it works
1. There are [tailers](https://github.com/Boiler/logcarrier/tree/master/tail) sending log data to logcarrier server. You can also found this tailing script in this repository.
2. `logcarrier` server what receives these files and save them under arbitrary names, optionally making links to them and:
    1. optional ZSTD compression
    2. mandatory logrotation at customizable schedule

# Features
1. Configured file names based on input parameters. Different setups for "original" and "log rotated" file names.
2. Link on files with similar features as on regular files.
3. ZSTD on the fly compression (can be memory hungry).
4. Log rotation setup. `periodic` is the recommended way, other methods are for compatibility reasons with old app.
5. Use YAML instead of original TOML for config. TOML can be easier to parse but its Go library is pretty poor in error handling and TOML itself doesn't support octal numbers (to describe directory rights) and things like `128Kb` at place.

# Stability and code quality
1. The core was written in a couple of days in a hurry, thus some design choices are questionable (I would call them wrong in fact) and to be rewritten. This won't reflect in functionality and usage though.
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
  zstdict: 128Kb                 # ZSTD compression dictionary size. They say this improves compression speed.
  connections: 1024              # Queue of jobs to accept incoming connections 
  dumps: 512                     # Queue of jobs to dump data from tailers
  logrotates: 512                # Queue of jobs to rotate log and its link 

workers:
  route: 128                     # How many workers to accept incoming connections
  dumper: 128                    # How many workers to save data from tailers
  logrotater: 12                 # How many workers to rotate logs and their links

  flusher_sleep: 30s             # Intervals for force flush

files:
  root: /var/logs/logcarrier                  # Root directory for file storage
  root_mode: 0755                             # Mode for subdirectories creating in a process
  name: /$dir/$name-${ time | %Y%m%d%H }      # File name template. This is a good idea to give file an already rotated name 
                                              # (date, hour, minute, etc) and use link with "original" file name pointed at the  
                                              # currently writing part
  rotation: /$dir/$name-${ time | %Y%m%d%H }  # Rename to on rotation. This time the same name
  before:                        # Describes a queue to put just created file names in
    type: file                   # Only file is supported now.
    path: "/tmp/file_creation" 
  after:                         # Describes a queue to put just rotated file names in.
    type: file                   
    path: '/tmp/file_rotation'

links:                           # Same as with files
  root: ..
  root_mode: ..
  name: ..
  rotation: ..
  before:
    type: file
    path: '/tmp/link_creation'
  after:                         # Same as for files
    type: file
    path: '/tmp/link_rotation'

logrotate:
  method: periodic               # Can be `periodic`, `guided` (via protocol) and `both`
  schedule: "* */1 * * *"        # Log rotation start schedule
```

# What before and after notifies are for?

They are used to signal what file/link is started to feeding up and what file/link is logrotated and will not be fed up
anymore. This can be used for tailing where we read from `before` queue to know files that are filling up now and stop
tailing ones that are enlisted in the `after` queue.
