package container

type CreateNginxSandboxContainerInput struct {
	Name  string
	Binds []string
}

type CreateNginxSandboxContainerOutput struct {
	ID   string
	Port string
	Host string
}

type RemoveContainerInput struct {
	ID string
}
