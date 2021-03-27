package localconf

import (
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

const (
	// InitSegmentName is segment name for ffmpeg command
	InitSegmentName string = "init-stream"
)

// Config struct for store command arguments and it's derived objects
type Config struct {
	Addr      *string
	Tmpl      *string
	Cmd       *string
	Static    *string
	WorkDir   *string
	TrustedIP *string
	Port      *uint

	regexpIP []*regexp.Regexp
	cmd      map[string]*template.Template
}

func (c *Config) parsePort() error {
	c.Port = new(uint)
	addr := *c.Addr
	colon := strings.LastIndex(addr, ":")
	if colon == -1 {
		i, e := strconv.Atoi(addr)
		if e == nil {
			*c.Port = uint(i)
		} else {
			return e
		}
	} else {
		i, e := strconv.Atoi(addr[colon+1:])
		if e == nil {
			*c.Port = uint(i)
		} else {
			return e
		}
	}
	return nil
}

func (c *Config) parseIP() error {
	c.regexpIP = make([]*regexp.Regexp, 0)
	array := strings.Split(*c.TrustedIP, ";")

	for _, ip := range array {
		if len(ip) == 0 {
			continue
		}
		re, err := regexp.Compile(ip)
		if err == nil {
			c.regexpIP = append(c.regexpIP, re)
		}
	}

	if len(c.regexpIP) == 0 {
		return fmt.Errorf("trustedIP list is empty")
	}
	return nil
}

func (c *Config) parseCmd() error {
	c.cmd = make(map[string]*template.Template)
	files, errReadDir := ioutil.ReadDir(*c.Cmd)
	if errReadDir != nil {
		return errReadDir
	}

	for _, f := range files {
		p, errPath := filepath.Abs(filepath.Join(*c.Cmd, f.Name()))
		if errPath != nil {
			return errPath
		}
		tmpl, errParseFiles := template.ParseFiles(p)
		if errParseFiles != nil {
			return errParseFiles
		}
		c.cmd[f.Name()] = tmpl
	}

	if len(c.cmd) == 0 {
		return fmt.Errorf("cmd list is empty")
	}
	return nil
}

// NewConfig build Config and derived objects from command arguments
func NewConfig(log *zap.Logger) *Config {
	c := new(Config)
	c.Addr = flag.String("addr", ":6060", "server listen addres")
	c.Tmpl = flag.String("tmpl", "tmpl", "http template directory path")
	c.Cmd = flag.String("cmd", "cmd", "command template directory path")
	c.Static = flag.String("static", "static", "static directory path")
	c.TrustedIP = flag.String("trustedIP", "127.0.0.1;", "tusted host - regexp: list of IP with any delimeter")
	c.WorkDir = flag.String("workDir", "ffmpeg", "work directory")
	flag.Parse()

	if err := c.parsePort(); err != nil {
		log.Error("parse port", zap.Error(err))
		return nil
	}

	if err := c.parseIP(); err != nil {
		log.Error("parse ip", zap.Error(err))
		return nil
	}

	if err := c.parseCmd(); err != nil {
		log.Error("parse cmd", zap.Error(err))
		return nil
	}

	errDir := os.MkdirAll(*c.WorkDir, os.ModePerm)
	if errDir != nil {
		log.Sugar().Error(errDir)
		return nil
	}

	log.Sugar().Warn("addr", *c.Addr)
	log.Sugar().Warn("tmpl", *c.Tmpl)
	log.Sugar().Warn("cmd", *c.Cmd)
	log.Sugar().Warn("static", *c.Static)
	log.Sugar().Warn("workDir", *c.WorkDir)
	log.Sugar().Warn("trustedIP", *c.TrustedIP)

	return c
}

// IsTrustedIP is check input ip with trusted
func (c *Config) IsTrustedIP(ip string) bool {
	end := strings.LastIndex(ip, ":")
	if end != -1 {
		ip = ip[0:end]
	}

	for _, re := range c.regexpIP {
		if re.MatchString(ip) {
			return true
		}
	}
	return false
}

// GetTmpl return command template by key
func (c *Config) GetTmpl(key string) (*template.Template, bool) {
	res, ok := c.cmd[key]
	return res, ok
}
