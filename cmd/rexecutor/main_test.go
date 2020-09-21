package main

import (
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sebidude/gintest"
	"github.com/stretchr/testify/assert"
)

var (
	config = &Configuration{
		ListenAddress: ":8080",
		Endpoints: []*EndpointConfig{
			&EndpointConfig{
				Path:       "/test",
				AllowMulti: false,
				Command:    "echo",
				Args:       []string{"Test Output"},
			},
			&EndpointConfig{
				Path:       "/testfail",
				AllowMulti: false,
				Command:    "echoo",
				Args:       []string{"Test Output"},
			},

			&EndpointConfig{
				Path:       "/testfailtimeout",
				AllowMulti: false,
				Command:    "curl",
				Args: []string{
					"--fail",
					"--connect-timeout ", "1",
					"localhostnot",
				},
			},
			&EndpointConfig{
				Path:       "/longrun",
				AllowMulti: false,
				Command:    "sleep",
				Args:       []string{"1"},
			},
		},
	}
)

func TestRunCommand(t *testing.T) {
	rce := new(Rexecutor)
	rce.Config = config
	rce.Jobs = make(Jobs)

	t.Run("run command success", func(t *testing.T) {
		handler := rce.runCommand(config.Endpoints[0])
		c, _ := gintest.PrepareEmptyRecordingContext()
		handler(c)
		assert.False(t, c.IsAborted(), "Context must not be aborted.")
	})

	t.Run("run command not found", func(t *testing.T) {
		handler := rce.runCommand(config.Endpoints[1])
		c, _ := gintest.PrepareEmptyRecordingContext()
		handler(c)
		assert.True(t, c.IsAborted(), "Context must be aborted.")
	})

	t.Run("run command fail", func(t *testing.T) {
		handler := rce.runCommand(config.Endpoints[2])
		c, _ := gintest.PrepareEmptyRecordingContext()
		handler(c)
		assert.True(t, c.IsAborted(), "Context must be aborted.")
	})

	t.Run("run twice fail", func(t *testing.T) {
		var wg sync.WaitGroup
		handler1 := rce.runCommand(config.Endpoints[3])
		handler2 := rce.runCommand(config.Endpoints[3])
		c1, _ := gintest.PrepareEmptyRecordingContext()
		c2, _ := gintest.PrepareEmptyRecordingContext()
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler1(c1)
		}()
		handler2(c2)
		wg.Wait()
		assert.True(t, c1.IsAborted(), "Context must be aborted.")
	})

}

func TestReloadConfig(t *testing.T) {
	rce := new(Rexecutor)
	rce.Router = gin.New()
	rce.Config = config
	rce.Jobs = make(Jobs)

	t.Run("successfull reload", func(t *testing.T) {
		configfile = "../../config.yaml"
		c, _ := gintest.PrepareEmptyRecordingContext()
		rce.reload(c)
		assert.Equal(t, "/fullBackup", rce.Config.Endpoints[0].Path, "Path must be updated.")
	})

	t.Run("missing configfile", func(t *testing.T) {
		configfile = "not found config.yaml"
		c, _ := gintest.PrepareEmptyRecordingContext()
		rce.reload(c)
		assert.True(t, c.IsAborted(), "Context must be aborted")
	})
}
