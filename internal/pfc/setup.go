package pfc

import (
	"fmt"
	"os/exec"

	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupNIC adds the PFC components to nic.
func SetupNIC(nic string, direction string, qid int, flags int) error {
	var err error
	log := ctrl.Log.WithName(nic)

	// tc qdisc add dev nic clsact
	err = addQueueDiscipline(nic)
	if err == nil {
		log.Info("qdisc added")
	} else {
		log.Error(err, "qdisc add error")
	}

	// tc filter add dev nic ingress bpf direct-action object-file pfc_ingress_tc.o sec .text
	err = addFilter(nic, direction)
	if err == nil {
		log.Info("filter added", "direction", direction)
	} else {
		log.Error(err, "ingress filter add error")
	}

	// ./cli_cfg set nic 0 0 9 "nic rx"
	err = configurePFC(nic, qid, flags)
	if err == nil {
		log.Info("pfc configured", "qid", qid, "flags", flags)
	} else {
		log.Error(err, "pfc configuration error")
	}

	return nil
}

func addQueueDiscipline(nic string) error {
	// add the clsact qdisc to the nic if it's not there
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("tc qdisc list dev %[1]s clsact | grep clsact || tc qdisc add dev %[1]s clsact", nic))
	return cmd.Run()
}

func addFilter(nic string, direction string) error {
	// add the pfc ingress filter to the nic if it's not already there
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("tc filter show dev %[1]s %[2]s | grep pfc_%[2]s_tc || tc filter add dev %[1]s %[2]s bpf direct-action object-file /opt/acnodal/bin/pfc_%[2]s_tc.o sec .text", nic, direction))
	return cmd.Run()
}

func configurePFC(nic string, qid int, flags int) error {
	// configure the PFC only if it hasn't been already
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("/opt/acnodal/bin/cli_cfg get %[1]s | grep GUE-DECAP || /opt/acnodal/bin/cli_cfg set %[1]s %[2]d 0 %[3]d \"%[1]s rx\"", nic, qid, flags))
	return cmd.Run()
}
