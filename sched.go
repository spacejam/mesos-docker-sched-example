package main

import (
	"flag"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"os"
	"time"

	log "github.com/golang/glog"
	mesos "github.com/mesos/mesos-go/mesosproto"
	util "github.com/mesos/mesos-go/mesosutil"
	sched "github.com/mesos/mesos-go/scheduler"
)

var (
	DOCKER_IMAGE_DEFAULT = "busybox"
	master               = flag.String("master", "127.0.0.1:5050", "Master address <ip:port>")
)

type Scheduler struct {
	tasksLaunched int
	tasksFinished int
	totalTasks    int
}

func NewScheduler() (*Scheduler, error) {
	s := Scheduler{}
	return &s, nil
}

func (sched *Scheduler) Registered(driver sched.SchedulerDriver, frameworkId *mesos.FrameworkID, masterInfo *mesos.MasterInfo) {
	log.Infoln("Scheduler Registered with Master ", masterInfo)
}

func (sched *Scheduler) Reregistered(driver sched.SchedulerDriver, masterInfo *mesos.MasterInfo) {
	log.Infoln("Scheduler Re-Registered with Master ", masterInfo)
}

func (sched *Scheduler) Disconnected(sched.SchedulerDriver) {
	log.Infoln("Scheduler Disconnected")
}

func (sched *Scheduler) ResourceOffers(driver sched.SchedulerDriver, offers []*mesos.Offer) {
	for _, offer := range offers {
		taskId := &mesos.TaskID{
			Value: proto.String(fmt.Sprintf("basicdocker-task-%d", time.Now().Unix())),
		}

		ports := util.FilterResources(
			offer.Resources,
			func(res *mesos.Resource) bool {
				return res.GetName() == "ports"
			},
		)
		if len(ports) > 0 && len(ports[0].GetRanges().GetRange()) > 0 {

		} else {
			return
		}
		task := &mesos.TaskInfo{
			Name:    proto.String(taskId.GetValue()),
			TaskId:  taskId,
			SlaveId: offer.SlaveId,
			Container: &mesos.ContainerInfo{
				Type:     mesos.ContainerInfo_DOCKER.Enum(),
				Volumes:  nil,
				Hostname: nil,
				Docker: &mesos.ContainerInfo_DockerInfo{
					Image:   &DOCKER_IMAGE_DEFAULT,
					Network: mesos.ContainerInfo_DockerInfo_BRIDGE.Enum(),
				},
			},
			Command: &mesos.CommandInfo{
				Shell: proto.Bool(true),
				Value: proto.String("set -x ; /bin/date ; /bin/hostname ; sleep 200 ; echo done"),
			},
			Executor: nil,
			Resources: []*mesos.Resource{
				util.NewScalarResource("cpus", getOfferCpu(offer)),
				util.NewScalarResource("mem", getOfferMem(offer)),
				util.NewRangesResource("ports", []*mesos.Value_Range{
					util.NewValueRange(
						*ports[0].GetRanges().GetRange()[0].Begin,
						*ports[0].GetRanges().GetRange()[0].Begin+1,
					),
				}),
			},
		}

		log.Infof("Prepared task: %s with offer %s for launch\n", task.GetName(), offer.Id.GetValue())

		var tasks []*mesos.TaskInfo = []*mesos.TaskInfo{task}
		log.Infoln("Launching ", len(tasks), " tasks for offer", offer.Id.GetValue())
		driver.LaunchTasks([]*mesos.OfferID{offer.Id}, tasks, &mesos.Filters{RefuseSeconds: proto.Float64(1)})
		sched.tasksLaunched++
		time.Sleep(time.Second)
	}
}

func (sched *Scheduler) StatusUpdate(driver sched.SchedulerDriver, status *mesos.TaskStatus) {
	log.Infoln("Status update: task", status.TaskId.GetValue(), " is in state ", status.State.Enum().String())

	if status.GetState() == mesos.TaskState_TASK_FINISHED {
		sched.tasksFinished++
		log.Infoln("%v of %v tasks finished.", sched.tasksFinished, sched.totalTasks)
	}
}

func (sched *Scheduler) OfferRescinded(s sched.SchedulerDriver, id *mesos.OfferID) {
	log.Infof("Offer '%v' rescinded.\n", *id)
}

func (sched *Scheduler) FrameworkMessage(s sched.SchedulerDriver, exId *mesos.ExecutorID, slvId *mesos.SlaveID, msg string) {
	log.Infof("Received framework message from executor '%v' on slave '%v': %s.\n", *exId, *slvId, msg)
}

func (sched *Scheduler) SlaveLost(s sched.SchedulerDriver, id *mesos.SlaveID) {
	log.Infof("Slave '%v' lost.\n", *id)
}

func (sched *Scheduler) ExecutorLost(s sched.SchedulerDriver, exId *mesos.ExecutorID, slvId *mesos.SlaveID, i int) {
	log.Infof("Executor '%v' lost on slave '%v' with exit code: %v.\n", *exId, *slvId, i)
}

func (sched *Scheduler) Error(driver sched.SchedulerDriver, err string) {
	log.Infoln("Scheduler received error:", err)
}

func init() {
	flag.Parse()
}

func main() {

	log.Info("Creating scheduler")
	scheduler, err := NewScheduler()
	if err != nil {
		log.Fatalf("Unable to create scheduler: %s\n", err.Error())
		os.Exit(1)
	}
	log.Infof("Created scheduler %+v\n", scheduler)

	log.Info("Creating framework info")
	fwinfo := &mesos.FrameworkInfo{
		User: proto.String(""),
		Name: proto.String("basicdocker-2"),
	}
	log.Infof("Created fwinfo %+v\n", fwinfo)

	log.Info("Creating scheduler driver config")
	config := sched.DriverConfig{
		Scheduler:  scheduler,
		Framework:  fwinfo,
		Master:     *master,
		Credential: (*mesos.Credential)(nil),
	}
	log.Infof("Created driver config %+v\n", config)

	log.Info("Creating new scheduler driver from config")
	driver, err := sched.NewMesosSchedulerDriver(config)

	if err != nil {
		log.Fatalf("Unable to create a SchedulerDriver: %v\n", err.Error())
		os.Exit(3)
	}
	log.Infof("Created scheduler driver %+v\n", driver)

	log.Info("Starting scheduler driver")
	if stat, err := driver.Run(); err != nil {
		log.Fatalf("Framework stopped with status %s and error: %s\n", stat.String(), err.Error())
		os.Exit(4)
	}
}
func getOfferScalar(offer *mesos.Offer, name string) float64 {
	resources := util.FilterResources(offer.Resources, func(res *mesos.Resource) bool {
		return res.GetName() == name
	})

	value := 0.0
	for _, res := range resources {
		value += res.GetScalar().GetValue()
	}

	return value
}

func getOfferCpu(offer *mesos.Offer) float64 {
	return getOfferScalar(offer, "cpus")
}

func getOfferMem(offer *mesos.Offer) float64 {
	return getOfferScalar(offer, "mem")
}

func logOffers(offers []*mesos.Offer) {
	for _, offer := range offers {
		log.Infof("Received Offer <%v> with cpus=%v mem=%v", offer.Id.GetValue(), getOfferCpu(offer), getOfferMem(offer))
	}
}
