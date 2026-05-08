package fleet

import "fmt"

type Fleet struct {
	Name      string      `json:"name"`
	Remote    string      `json:"remote"`
	Instances []*Instance `json:"instances"`
}

func (f *Fleet) GetInstance(name string) (*Instance, error) {
	for _, instance := range f.Instances {
		if instance.Name == name {
			return instance, nil
		}
	}
	return nil, fmt.Errorf("instance %q not found in fleet %q", name, f.Name)
}

func (f *Fleet) AddInstance(instance *Instance) error {
	for _, existing := range f.Instances {
		if existing.Name == instance.Name {
			return fmt.Errorf("instance %q already exists in fleet %q", instance.Name, f.Name)
		}
	}
	f.Instances = append(f.Instances, instance)
	return nil
}

func (f *Fleet) RemoveInstance(name string) error {
	for i, instance := range f.Instances {
		if instance.Name == name {
			f.Instances = append(f.Instances[:i], f.Instances[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("instance %q not found in fleet %q", name, f.Name)
}
