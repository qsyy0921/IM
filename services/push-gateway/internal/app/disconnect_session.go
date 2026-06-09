package app

type DisconnectSessionUseCase struct {
	registry SessionRegistry
}

func NewDisconnectSessionUseCase(registry SessionRegistry) *DisconnectSessionUseCase {
	return &DisconnectSessionUseCase{registry: registry}
}

func (usecase *DisconnectSessionUseCase) Execute(sessionID string) {
	if sessionID == "" {
		return
	}
	usecase.registry.Unregister(sessionID)
}
