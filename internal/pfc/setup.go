package pfc

import (
	"fmt"

	egwexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupNIC adds the PFC components to nic.
func SetupNIC(nic string, function string, direction string, qid int, flags int) error {
	var err error
	log := ctrl.Log.WithName(nic)

	// tc qdisc add dev nic clsact
	err = AddQueueDiscipline(nic)
	if err == nil {
		log.Info("qdisc added")
	} else {
		log.Error(err, "qdisc add error")
	}

	// tc filter add dev nic ingress bpf direct-action object-file pfc_ingress_tc.o sec .text
	err = AddFilter(nic, direction, function)
	if err == nil {
		log.Info("filter added", "function", function, "direction", direction)
	} else {
		log.Error(err, "ingress filter add error")
	}

	// ./cli_cfg set nic 0 0 9 "nic rx"
	err = configurePFC(nic, qid, flags, direction)
	if err == nil {
		log.Info("pfc configured", "qid", qid, "flags", flags)
	} else {
		log.Error(err, "pfc configuration error")
	}

	return nil
}

// AddQueueDiscipline adds a clsact queue discipline to the specified
// NIC.
func AddQueueDiscipline(nicName string) error {
	// add the clsact qdisc to the nic if it's not there
	script := fmt.Sprintf("/usr/sbin/tc qdisc list dev %[1]s clsact | grep clsact || /usr/sbin/tc qdisc add dev %[1]s clsact", nicName)
	return egwexec.RunScript(ctrl.Log, script)
}

// AddFilter adds the pfc filter to the nic if it's not already there.
func AddFilter(nic string, direction string, filter string) error {
	script := fmt.Sprintf("/usr/sbin/tc filter show dev %[1]s %[2]s | grep pfc_%[3]s_tc || /usr/sbin/tc filter add dev %[1]s %[2]s bpf direct-action object-file /opt/acnodal/bin/pfc_%[3]s_tc.o sec .text", nic, direction, filter)
	return egwexec.RunScript(ctrl.Log, script)
}

func configurePFC(nic string, qid int, flags int, direction string) error {
	// configure the PFC
	script := fmt.Sprintf("/opt/acnodal/bin/cli_cfg set %[1]s %[2]d %[3]d \"%[1]s %[4]s\"", nic, qid, flags, direction)
	return egwexec.RunScript(ctrl.Log, script)
}
