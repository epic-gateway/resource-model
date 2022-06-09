package controllers

import (
	"fmt"

	"github.com/go-logr/logr"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	epicexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
)

func configureTunnel(l logr.Logger, ep epicv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel get %[1]d || /opt/acnodal/bin/cli_tunnel set %[1]d %[2]s %[3]d 0 0", ep.TunnelID, ep.Address, ep.Port.Port)
	return epicexec.RunScript(l, script)
}

func configureService(l logr.Logger, ep epicv1.RemoteEndpointSpec, ifindex int, tunnelID uint32, tunnelAuth string) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service set-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", tunnelID>>16, tunnelID&0xffff, tunnelAuth, tunnelID, ep.Address, ep.Port.Port, ifindex)
	return epicexec.RunScript(l, script)
}
