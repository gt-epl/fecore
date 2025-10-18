// TODO starter code, needs to be rearrange and integrated

package storage

type Function struct {
	Name        string // unique
	Namespace   string
	Image       string
	Labels      string // json
	Annotations string // json
	Secrets     string // []string
	SecretsPath string
	EnvVars     string // json
	EnvProcess  string
	MemoryLimit int64
}

type Container struct {
	Name           string // unique
	ParentFunction string
	Ip             string
}

type StorageManager interface {
	InsertFunction(function Function) error
	GetAllFunctions() ([]Function, error)
	DeleteFunction(name string) error

	InsertContainer(container Container) error
	GetContainersForFunction(name string) ([]Container, error)
	GetAllContainers() ([]Container, error)
	DeleteContainer(name string) error
}
