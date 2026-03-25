package fleet

import "fmt"

type Fleet struct {
	Name      string      `json:"name"`
	Remote    string      `json:"remote"`
	Instances []*Instance `json:"instances"`
}

func (f *Fleet) GetInstance(name string) (*Instance, error) {
	for _, inst := range f.Instances {
		if inst.Name == name {
			return inst, nil
		}
	}
	return nil, fmt.Errorf("instance %q not found in fleet %q", name, f.Name)
}

func (f *Fleet) AddInstance(inst *Instance) error {
	for _, existing := range f.Instances {
		if existing.Name == inst.Name {
			return fmt.Errorf("instance %q already exists in fleet %q", inst.Name, f.Name)
		}
	}
	f.Instances = append(f.Instances, inst)
	return nil
}

func (f *Fleet) RemoveInstance(name string) error {
	for i, inst := range f.Instances {
		if inst.Name == name {
			f.Instances = append(f.Instances[:i], f.Instances[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("instance %q not found in fleet %q", name, f.Name)
}
