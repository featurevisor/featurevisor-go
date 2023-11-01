package featurevisor

import "github.com/featurevisor/featurevisor-go/instance"

func NewInstance(datafileURL string) (*instance.Instance, error) {
	factory := &instance.Factory{
		DatafileURL: datafileURL,
	}

	return factory.NewInstance()
}
