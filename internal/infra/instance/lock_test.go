package instance

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestWorkspaceIsExclusiveAcrossProcesses(t *testing.T) {
	if root := os.Getenv("NAVIFYNE_LOCK_TEST_CHILD"); root != "" {
		lock, err := Acquire(root)
		if !errors.Is(err, ErrBusy) {
			if lock != nil {
				lock.Close()
			}
			t.Fatal("child could acquire an occupied workspace")
		}
		return
	}
	root := t.TempDir()
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestWorkspaceIsExclusiveAcrossProcesses$")
	command.Env = append(os.Environ(), "NAVIFYNE_LOCK_TEST_CHILD="+root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child lock check: %s %v", output, err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(root)
	if err != nil {
		t.Fatal("lock was not released", err)
	}
	next.Close()
}
