package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/dchest/uniuri"
	"github.com/gin-gonic/gin"
	"github.com/sebidude/configparser"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

type CommandProperties struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Configuration struct {
	ListenAddress string                       `json:"listenAddress"`
	CommandMap    map[string]CommandProperties `json:"commands"`
}

type Rexecutor struct {
	Config *Configuration
}

var (
	appconfig  Configuration
	configfile string
)

func main() {

	log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.LUTC | log.Ldate)

	app := kingpin.New("rexecutor", "Run command on remote")
	app.Flag("config", "Full path to the configfile.").Short('c').Required().StringVar(&configfile)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	err := configparser.ParseYaml(configfile, &appconfig)
	if err != nil {
		log.Println(configfile)
		log.Fatal(err)
	}
	configparser.SetValuesFromEnvironment("RCE", &appconfig)

	rce := &Rexecutor{}
	rce.Config = &appconfig

	log.Println("starting rexecutor")
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(GinLogger())
	router.GET("/cmd/:cmd", rce.runCommand)
	err = router.Run(rce.Config.ListenAddress)
	if err != nil {
		log.Fatalln(err.Error())
	}
	log.Println("Shutdown.")
	os.Exit(0)

}

func (a *Rexecutor) runCommand(c *gin.Context) {
	rid := uniuri.NewLen(8)
	c.Set("id", rid)
	cmdparam := c.Param("cmd")
	if _, ok := a.Config.CommandMap[cmdparam]; ok {
		cmdargs := a.Config.CommandMap[cmdparam].Args
		cmdstr := a.Config.CommandMap[cmdparam].Command

		log.Printf("%s - Found cmd for: %s -> %s", rid, cmdparam, cmdstr)
		cmd := exec.Command(cmdstr, cmdargs...)
		c.Stream(func(w io.Writer) bool {
			cmdout, _ := cmd.StdoutPipe()
			cmd.Start()
			io.Copy(w, cmdout)
			cmd.Wait()
			return false
		})
		log.Printf("%s - done.", rid)
	} else {
		c.String(http.StatusNotFound, "no such command.")
		return
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

		log.Println(logstring)

	}
}
