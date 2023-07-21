package exec

import (
	"os/exec"

	"github.com/go-logr/logr"
)

// RunScript runs the script in /bin/sh. If the command fails then it
// logs the combined stdout and stderr.
func RunScript(log logr.Logger, script string) error {
	var err error

	cmd := exec.Command("/bin/sh", "-c", script)
	if stdoutStderr, err := cmd.CombinedOutput(); err != nil {
		log.Error(err, "Script FAILED", "script", script, "combined-output", string(stdoutStderr))
	} else {
		log.V(1).Info("Script succeeded", "script", script)
	}

	return err
}
