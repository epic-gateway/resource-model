package pfc

import (
	"fmt"

	"github.com/go-logr/logr"
	epicexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
)

// SetupNIC adds the PFC components to nic.
func SetupNIC(logger logr.Logger, nic string, function string, direction string, qid int, flags int) error {
	var err error
	log := logger.WithValues("interface", nic)

	// tc qdisc add dev nic clsact
	err = AddQueueDiscipline(log, nic)
	if err == nil {
		log.V(1).Info("qdisc added")
	} else {
		log.Error(err, "qdisc add error")
	}

	// tc filter add dev nic ingress bpf direct-action object-file pfc_ingress_tc.o sec .text
	err = AddFilter(log, nic, direction, function)
	if err == nil {
		log.V(1).Info("filter added", "function", function, "direction", direction)
	} else {
		log.Error(err, "ingress filter add error")
	}

	// ./cli_cfg set nic 0 0 9 "nic rx"
	err = configurePFC(log, nic, qid, flags, direction)
	if err == nil {
		log.V(1).Info("pfc configured", "qid", qid, "flags", flags)
	} else {
		log.Error(err, "pfc configuration error")
	}

	return nil
}

// AddQueueDiscipline adds a clsact queue discipline to the specified
// NIC.
func AddQueueDiscipline(log logr.Logger, nicName string) error {
	// add the clsact qdisc to the nic if it's not there
	script := fmt.Sprintf("/usr/sbin/tc qdisc list dev %[1]s clsact | grep clsact || /usr/sbin/tc qdisc add dev %[1]s clsact", nicName)
	return epicexec.RunScript(log, script)
}

// AddFilter adds the pfc filter to the nic if it's not already there.
func AddFilter(log logr.Logger, nic string, direction string, filter string) error {
	script := fmt.Sprintf("/usr/sbin/tc filter show dev %[1]s %[2]s | grep pfc_%[3]s_tc || /usr/sbin/tc filter add dev %[1]s %[2]s bpf direct-action object-file /opt/acnodal/bin/pfc_%[3]s_tc.o sec .text", nic, direction, filter)
	return epicexec.RunScript(log, script)
}

func configurePFC(log logr.Logger, nic string, qid int, flags int, direction string) error {
	// configure the PFC
	script := fmt.Sprintf("/opt/acnodal/bin/cli_cfg set %[1]s %[2]d %[3]d \"%[1]s %[4]s\"", nic, qid, flags, direction)
	return epicexec.RunScript(log, script)
}
