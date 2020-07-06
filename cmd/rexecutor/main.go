package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/dchest/uniuri"
	"github.com/gin-gonic/gin"
	"github.com/sebidude/configparser"
	log "github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

type Job struct {
	JobID      string          `json:"jobID" yaml:"jobID"`
	Endpoint   *EndpointConfig `json:"endpoint" yaml:"endpoint"`
	ExitCode   int             `json:"exitCode" yaml:"exitCode"`
	Running    bool            `json:"running" yaml:"running"`
	OutputPipe bytes.Buffer    `json:"-"`
	Output     string          `json:"output" yaml:"output"`
	Pid        int             `json:"pid" yaml:"pid"`
}

type Jobs map[string]*Job

type EndpointConfig struct {
	Path       string   `json:"path" yaml:"path"`
	AllowMulti bool     `json:"allowMulti" yaml:"allowMulti"`
	Command    string   `json:"command" yaml:"command"`
	Args       []string `json:"args" yaml:"command"`
}

type Configuration struct {
	ListenAddress string            `json:"listenAddress" yaml:"listenAddress"`
	Endpoints     []*EndpointConfig `json:"endpoints" yaml:"endpoints"`
}

type Rexecutor struct {
	Config   *Configuration
	Router   *gin.Engine
	Jobs     Jobs
	JobsDone chan string
}

var (
	appconfig  Configuration
	configfile string
	appversion string
	gitcommit  string
	buildtime  string
)

func main() {

	log.SetFormatter(&nested.Formatter{
		HideKeys:        true,
		FieldsOrder:     []string{"component", "category"},
		TimestampFormat: time.RFC3339Nano,
	})

	app := kingpin.New("rexecutor", "Run command on remote")
	app.Flag("config", "Full path to the configfile.").Short('c').Default("config.yaml").StringVar(&configfile)

	kingpin.MustParse(app.Parse(os.Args[1:]))
	log.WithField("component", "main").Infof("==== Rexecutor ====")
	log.WithField("component", "main").Infof("appversion: %s", appversion)
	log.WithField("component", "main").Infof("gitcommit:  %s", gitcommit)
	log.WithField("component", "main").Infof("buildtime:  %s", buildtime)
	log.WithField("component", "main").Infof("Reading configfile: %s", configfile)

	err := configparser.ParseYaml(configfile, &appconfig)
	if err != nil {
		log.Println(configfile)
		log.Fatal(err)
	}
	log.WithField("component", "main").Infof("Setting config parameters from env.")
	configparser.SetValuesFromEnvironment("RCE", &appconfig)

	rce := new(Rexecutor)
	rce.Config = &appconfig
	rce.Jobs = make(Jobs)

	gin.SetMode(gin.ReleaseMode)
	rce.Router = gin.New()
	rce.Router.Use(gin.Recovery())
	rce.Router.Use(GinLogger())
	log.WithField("component", "router").Infof("setting up endpoints")
	for _, endpoint := range rce.Config.Endpoints {
		log.WithField("component", "router").Infof("endpoint: %#v", endpoint)
		rce.Router.GET("/run/"+endpoint.Path, rce.runCommand(endpoint))
	}
	rce.Router.GET("/output/:jobid", rce.jobOutput)
	rce.Router.GET("/result/:jobid", rce.jobResult)
	rce.Router.GET("/status/:jobid", rce.jobStatus)
	rce.Router.POST("/reload", rce.reload)
	log.WithField("component", "main").Infof("Start server on %s", rce.Config.ListenAddress)

	err = rce.Router.Run(rce.Config.ListenAddress)
	if err != nil {
		log.Fatalln(err.Error())
	}
	log.Println("Shutdown.")
	os.Exit(0)

}

func (rce *Rexecutor) runCommand(endpoint *EndpointConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := uniuri.NewLen(8)
		c.Set("id", rid)
		for _, v := range rce.Jobs {
			if v.Endpoint.Path == endpoint.Path && !v.Endpoint.AllowMulti {
				log.WithField("component", "runner").Errorf("%s - A job for %s is already running", rid, endpoint.Path)
				c.String(409, "A job for this endpoint is already running.")
				c.Abort()
				return
			}
		}

		job := new(Job)
		job.Endpoint = endpoint
		job.JobID = rid
		rce.Jobs[rid] = job

		log.WithField("component", "runner").Infof("%s - exec: %#v", rid, endpoint)
		cmd := exec.Command(endpoint.Command, endpoint.Args...)
		cmdout, _ := cmd.StdoutPipe()
		jw := bufio.NewWriter(&job.OutputPipe)
		job.Running = true
		cmd.Start()
		io.Copy(jw, cmdout)
		job.Pid = cmd.Process.Pid
		if err := cmd.Wait(); err != nil {
			log.WithField("component", "runner").Errorf("%s - job failed: %s", rid, err.Error())
			c.String(500, "Job %s failed: %s\n%s", rid, err.Error(), job.OutputPipe.String())
			c.Abort()
			delete(rce.Jobs, rid)
			return
		}
		job.Running = false
		job.ExitCode = cmd.ProcessState.ExitCode()
		log.WithField("component", "runner").Infof("%s - exec: %#v", rid, endpoint)
		c.String(200, "Job %s finished.\n%s", rid, job.OutputPipe.String())
		delete(rce.Jobs, rid)
	}
}

func (rce *Rexecutor) reload(c *gin.Context) {
	rid := uniuri.NewLen(8)
	c.Set("id", rid)
	log.WithField("component", "main").Infof("Reloading configfile.")
	err := configparser.ParseYaml(configfile, &appconfig)
	if err != nil {
		log.Println(configfile)
		log.WithField("component", "main").Errorf("Failed to reload config: %s", err.Error())
		c.AbortWithError(500, err)
		return
	}
	log.WithField("component", "main").Infof("Setting config parameters from env.")
	configparser.SetValuesFromEnvironment("RCE", &appconfig)
	rce.Config = &appconfig
	for _, endpoint := range rce.Config.Endpoints {
		log.WithField("component", "router").Infof("endpoint: %#v", endpoint)
	}
	log.WithField("component", "main").Infof("Config reloaded successfully.")

}

func (rce *Rexecutor) jobResult(c *gin.Context) {
	jobID := c.Param("jobid")
	rid := uniuri.NewLen(8)
	c.Set("id", rid)

	if j, ok := rce.Jobs[jobID]; ok {
		j.Output = j.OutputPipe.String()
		c.JSON(200, gin.H{
			"message":   "Job results",
			"requestID": rid,
			"job":       j,
		})
	} else {
		c.JSON(404, gin.H{
			"message":   "No job found",
			"requestID": rid,
			"jobID":     jobID,
		})
	}
}

func (rce *Rexecutor) jobOutput(c *gin.Context) {
	jobID := c.Param("jobid")
	rid := uniuri.NewLen(8)
	c.Set("id", rid)
	if j, ok := rce.Jobs[jobID]; ok {
		c.String(200, j.OutputPipe.String())
	} else {
		c.String(404, "No job found with id: %s", jobID)
	}
}

func (rce *Rexecutor) jobStatus(c *gin.Context) {
	jobID := c.Param("jobid")
	rid := uniuri.NewLen(8)
	c.Set("id", rid)
	if j, ok := rce.Jobs[jobID]; ok {
		if j.Running {
			c.String(200, "Running")
		} else {
			c.String(200, "Finished")
		}

	} else {
		c.String(404, "No job found with id: %s", jobID)
	}
}

func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()
		c.Next()
		rid, _ := c.Get("id")
		// after request
		latency := time.Since(t)

		// access the status we are sending
		status := c.Writer.Status()
		logstring := fmt.Sprintf("%s - %s - %d - %s (%s)",
			rid,
			c.Request.RemoteAddr,
			status,
			c.Request.RequestURI,
			latency)

		log.WithField("component", "router").Infof(logstring)

	}
}
