package worker

/*
	log_parser.go parse logs

	It especially defines a LogWorker type, that can read lines of the log,
	and convert them into JSON format. It shares a channel with the
	with which to communicate the JSON objects it finds.

	It uses the tail library to read the log.

	The worker configuation information is found in config.go.
*/
import (
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ActiveState/tail"
	"github.com/fizx/logs"
	"github.com/spf13/viper"
)

const configParseKeysToIgnore = "parse.keys_to_ignore"
const configParseTimePatterns = "parse.time_patterns"
const configParsePattern = "parse.pattern"
const configTailReopen = "time.reopen"
const configTail = "parse.time_patterns"

// DefaultParseLogPattern is the default pattern for understanding log patterns
const DefaultParseLogPattern = ""

// LogParser parses the imput and puts events on a channel
type LogParser struct {
	Config    *viper.Viper
	InputFile string
	Channel   chan map[string]interface{}
	tailer    *tail.Tail
	regex     *regexp.Regexp
}

func newKeyName(k string, m map[string]interface{}) string {
	name := k
	_, found := m[name]
	if !found {
		return name
	}
	return newKeyName("_"+k, m)
}

func sliceContains(list []string, a string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func (worker *LogParser) shouldIgnore(key string) bool {
	keysToIgnore := worker.Config.GetStringSlice(configParseKeysToIgnore)
	return key == "" || sliceContains(keysToIgnore, key)
}

func parseString(ts string, config *viper.Viper) interface{} {
	timePatterns := config.GetStringSlice(configParseTimePatterns)
	for _, timePattern := range timePatterns {
		t, e := time.Parse(timePattern, ts)
		if e == nil {
			return t
		}
	}
	nginxFormat := "02/Jan/2006:15:04:05 -0700"
	t, e := time.Parse(nginxFormat, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.ANSIC, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.UnixDate, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RubyDate, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RFC822, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RFC822Z, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RFC850, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RFC1123, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RFC1123Z, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RFC3339, ts)
	if e == nil {
		return t
	}
	t, e = time.Parse(time.RFC3339Nano, ts)
	if e == nil {
		return t
	}
	pi, err := strconv.ParseInt(ts, 10, 64)
	if err == nil {
		return pi
	}
	pb, err := strconv.ParseBool(ts)
	if err == nil {
		return pb
	}
	pf, err := strconv.ParseFloat(ts, 64)
	if err == nil {
		// it might be the string "NaN"
		if !math.IsNaN(pf) {
			return pf
		}
		return 0.0 // return 0.0 for NaN
	}
	return ts
}

// ParseURI parses the URI string and adds the relevant query parameters
// into the main map.
// it also attempts to determine the data type of the items by
// parsing as date, int, bool, float, and if all of these fail, then keeping
// as string
func (worker *LogParser) ParseURI(uri string, v map[string]interface{}) {
	if uri != "" {
		url, err := url.Parse(uri)
		if err == nil {
			q := url.Query()
			for k, kvs := range q {
				newKey := newKeyName(k, v)
				if !worker.shouldIgnore(newKey) && len(kvs) > 0 {
					v[newKey] = parseString(kvs[0], worker.Config)
				}
			}
		}
	}
}

// ParseEvents parses the line (including a call to ParseURI) to
// add events to the map of strings -> anything. It returns that map
func (worker *LogParser) ParseEvents(line string) (map[string]interface{}, error) {
	v := make(map[string]interface{})
	match := worker.regex.FindStringSubmatch(line)
	names := worker.regex.SubexpNames()
	if match != nil {
		for i, submatch := range match {
			name := names[i]
			if !worker.shouldIgnore(name) {
				v[names[i]] = parseString(submatch, worker.Config)
			}
			if name == "uri" {
				worker.ParseURI(submatch, v)
			}
		}
		return v, nil
	}
	logs.Debug("Line %s did not match pattern.", line)
	return nil, fmt.Errorf("Line %s did not match pattern.", line)
}

// converts worker config into tail Config
func (worker *LogParser) convertConfig() (config tail.Config) {
	config = tail.Config{}
	if !worker.Config.GetBool("tail.from_beginng") {
		config.Location = &tail.SeekInfo{0, os.SEEK_END}
	}
	if worker.Config.IsSet("tail.reopen") {
		config.ReOpen = worker.Config.GetBool(configTailReopen)
	}
	config.Follow = true
	config.Logger = tail.DiscardingLogger
	logs.Info("tail config: %v", config)
	return
}

// Init initializes worker's regex
func (worker *LogParser) Init() {
	pattern := worker.Config.GetString(configParsePattern)
	if pattern == "" {
		pattern = DefaultParseLogPattern
	}
	regex, err := regexp.Compile(pattern)
	if err != nil {
		logs.Warn("Could not compile Regex. Error: %v", err)
		return
	}
	worker.regex = regex
}

// Start starts the LogWorker.
// it starts tailing the log file, and parsing lines from it
// putting parsed lines on the shared channel.
func (worker *LogParser) Start() {
	logs.Info("Starting worker process")
	worker.Init()

	inputFile := worker.InputFile
	t, err := tail.TailFile(inputFile,
		worker.convertConfig())
	if err != nil {
		logs.Warn("Input file could not be opened: %s; error: %s", inputFile, err)

	} else {
		worker.tailer = t
		for line := range t.Lines {
			s := strings.TrimSpace(line.Text)
			logs.Debug("Processing line %v", s)
			v, err := worker.ParseEvents(s)
			if err == nil {
				go func() {
					worker.Channel <- v
				}()
			}
		}
	}
	logs.Info("Stopping worker process")
}

// Stop stops the worker and cleans up. Does *not* stop ElasticSearchWorker
func (worker *LogParser) Stop() {
	if worker.tailer != nil {
		worker.tailer.Stop()
		worker.tailer.Cleanup()
	}
}