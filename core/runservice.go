package core

import (
	"fmt"
	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"strings"
	"sync"
	"time"
)

// Note: The ServiceJob is loosely inspired by https://github.com/alexellis/jaas/

type RunServiceJob struct {
	BareJob
	Client              *docker.Client `json:"-"`
	User                string         `default:"root"`
	TTY                 bool           `default:"false"`
	Delete              bool           `default:"true"`
	Image               string
	Network             string
	Registry            string `default:""`
	LoggingGelfAddress  string `default:"" gcfg:"logging-gelf-address"`
	PlacementConstraint string `default:"" gcfg:"placement-constraint"`
}

func NewRunServiceJob(c *docker.Client) *RunServiceJob {
	return &RunServiceJob{Client: c}
}

func (j *RunServiceJob) Run(ctx *Context) error {
	if err := j.pullImage(); err != nil {
		return err
	}

	svc, err := j.buildService()

	if err != nil {
		return err
	}

	ctx.Logger.Noticef("Created service %s (%s) for job %s\n", svc.ID, j.InstanceName, j.Name)

	if err := j.watchContainer(ctx, svc.ID); err != nil {
		if err2 := j.deleteService(ctx, svc.ID); err2 != nil {
			ctx.Logger.Errorf("error deleting service %q: %s", fullImageName(j.Registry, j.Image), err2)
		}

		return err
	}

	return j.deleteService(ctx, svc.ID)
}

func (j *RunServiceJob) pullImage() error {
	o, a := buildPullOptions(j.Image, j.Registry)
	if err := j.Client.PullImage(o, a); err != nil {
		return fmt.Errorf("error pulling image %q: %s", fullImageName(j.Registry, j.Image), err)
	}

	return nil
}

func (j *RunServiceJob) buildService() (*swarm.Service, error) {

	//createOptions := types.ServiceCreateOptions{}

	max := uint64(1)
	createSvcOpts := docker.CreateServiceOptions{}

	j.InstanceName = fmt.Sprintf("%s_%d", j.Name, time.Now().Unix())

	createSvcOpts.ServiceSpec.Annotations.Name = j.InstanceName

	createSvcOpts.ServiceSpec.TaskTemplate.ContainerSpec =
		&swarm.ContainerSpec{
			Image: fullImageName(j.Registry, j.Image),
		}

	// Make the service run once and not restart
	createSvcOpts.ServiceSpec.TaskTemplate.RestartPolicy =
		&swarm.RestartPolicy{
			MaxAttempts: &max,
			Condition:   swarm.RestartPolicyConditionNone,
		}

	// For a service to interact with other services in a stack,
	// we need to attach it to the same network
	if j.Network != "" {
		createSvcOpts.Networks = []swarm.NetworkAttachmentConfig{
			swarm.NetworkAttachmentConfig{
				Target: j.Network,
			},
		}
	}

	if j.LoggingGelfAddress != "" {
		createSvcOpts.ServiceSpec.TaskTemplate.LogDriver =
			&swarm.Driver{
				Name:    "gelf",
				Options: map[string]string{"gelf-address": j.LoggingGelfAddress},
			}
	}

	if j.PlacementConstraint != "" {
		createSvcOpts.ServiceSpec.TaskTemplate.Placement =
			&swarm.Placement{
				Constraints: []string{j.PlacementConstraint},
			}
	}

	if j.Command != "" {
		createSvcOpts.ServiceSpec.TaskTemplate.ContainerSpec.Command = strings.Split(j.Command, " ")
	}

	svc, err := j.Client.CreateService(createSvcOpts)
	if err != nil {
		return nil, err
	}

	return svc, err
}

const (

	// TODO are these const defined somewhere in the docker API?
	swarmError   = -999
	timeoutError = -998
)

var svcChecker = time.NewTicker(watchDuration)

func (j *RunServiceJob) watchContainer(ctx *Context, svcID string) error {

	exitCode := swarmError

	ctx.Logger.Noticef("Checking for service ID %s (%s) termination\n", svcID, j.InstanceName)

	svc, err := j.Client.InspectService(svcID)
	if err != nil {
		return fmt.Errorf("Failed to inspect service %s: %s", svcID, err.Error())
	}

	// On every tick, check if all the services have completed, or have error out
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for _ = range svcChecker.C {

			if svc.CreatedAt.After(time.Now().Add(maxProcessDuration)) {
				err = ErrMaxTimeRunning
				return
			}

			taskExitCode, found := j.findTaskStatus(ctx, svc.ID)

			if found {
				exitCode = taskExitCode
				return
			}
		}
	}()

	wg.Wait()

	ctx.Logger.Noticef("Service ID %s (%s) has completed\n", svcID, j.InstanceName)

	switch exitCode {
	case 0:
		return nil
	default:
		return fmt.Errorf("exit code: %d", exitCode)
	}
}

func (j *RunServiceJob) findTaskStatus(ctx *Context, svcID string) (int, bool) {
	taskFilters := make(map[string][]string)
	taskFilters["service"] = []string{svcID}

	tasks, err := j.Client.ListTasks(docker.ListTasksOptions{
		Filters: taskFilters,
	})

	if err != nil {
		ctx.Logger.Errorf("Failed to find task ID %s. Considering the task terminated: %s\n", svcID, err.Error())
		return 0, false
	}

	if len(tasks) == 0 {
		// That task is gone now (maybe someone else removed it. Our work here is done
		return 0, true
	}

	exitCode := 1
	var done bool
	stopStates := []swarm.TaskState{
		swarm.TaskStateComplete,
		swarm.TaskStateFailed,
		swarm.TaskStateRejected,
	}

	for _, task := range tasks {

		stop := false
		for _, stopState := range stopStates {
			if task.Status.State == stopState {
				stop = true
				break
			}
		}

		if stop {

			exitCode = task.Status.ContainerStatus.ExitCode

			if exitCode == 0 && task.Status.State == swarm.TaskStateRejected {
				exitCode = 255 // force non-zero exit for task rejected
			}
			done = true
			break
		}
	}
	return exitCode, done
}

func (j *RunServiceJob) deleteService(ctx *Context, svcID string) error {
	if !j.Delete {
		return nil
	}

	err := j.Client.RemoveService(docker.RemoveServiceOptions{
		ID: svcID,
	})

	if _, is := err.(*docker.NoSuchService); is {
		ctx.Logger.Warningf("Service %s cannot be removed. An error may have happened, "+
			"or it might have been removed by another process", svcID)
		return nil
	}

	return err
}
