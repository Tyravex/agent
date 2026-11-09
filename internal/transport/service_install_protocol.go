package transport

import "github.com/Tyravex/agent/internal/transport/protocol"

type ServiceInstallEgg = protocol.ServiceInstallEgg
type ServiceInstallRuntime = protocol.ServiceInstallRuntime
type ServiceInstallResources = protocol.ServiceInstallResources
type ServiceInstallVariable = protocol.ServiceInstallVariable
type ServiceInstallPayload = protocol.ServiceInstallPayload
type ServiceInstallCommand = protocol.ServiceInstallCommand

func DecodeServiceInstallPayload(data []byte) (ServiceInstallPayload, error) {
	return protocol.DecodeServiceInstallPayload(data)
}

func ValidateServiceInstallPayload(payload ServiceInstallPayload) error {
	return protocol.ValidateServiceInstallPayload(payload)
}

func DecodeServiceInstallCommand(data []byte) (ServiceInstallCommand, error) {
	return protocol.DecodeServiceInstallCommand(data)
}

func validInstallCommandID(value string) bool {
	return protocol.ValidInstallCommandID(value)
}
