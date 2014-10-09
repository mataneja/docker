package volumes

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/pkg/log"
	"github.com/docker/docker/pkg/namesgenerator"
	"github.com/docker/docker/utils"
)

type Repository struct {
	configPath string
	driver     graphdriver.Driver
	volumes    map[string]*Volume
	nameIndex  map[string]*Volume
	idIndex    map[string]*Volume
	lock       sync.Mutex
}

func NewRepository(configPath string, driver graphdriver.Driver) (*Repository, error) {
	abspath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, err
	}

	// Create the config path
	if err := os.MkdirAll(abspath, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}

	repo := &Repository{
		driver:     driver,
		configPath: abspath,
		volumes:    make(map[string]*Volume),
		nameIndex:  make(map[string]*Volume),
		idIndex:    make(map[string]*Volume),
	}

	return repo, repo.restore()
}

func (r *Repository) newVolume(path, name string, writable bool) (*Volume, error) {
	var (
		isBindMount bool
		err         error
		id          string
	)

	id, err = r.generateNewId()

	if name == "" {
		name, err = r.generateNewName()
		if err != nil {
			return nil, err
		}
	}

	if v := r.findByName(name); v != nil {
		return nil, fmt.Errorf("Volume exists for name: %s", name)
	}

	if path != "" {
		isBindMount = true
	}

	if path == "" {
		path, err = r.createNewVolumePath(id)
		if err != nil {
			return nil, err
		}
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}

	if v := r.get(path); v != nil {
		return nil, fmt.Errorf("Volume exists for path: %s", path)
	}

	v := &Volume{
		ID:          id,
		Path:        path,
		repository:  r,
		Writable:    writable,
		containers:  make(map[string]struct{}),
		configPath:  r.configPath + "/" + id,
		IsBindMount: isBindMount,
		Created:     time.Now().UTC(),
		Name:        name,
	}

	if err := v.initialize(); err != nil {
		return nil, err
	}

	return v, r.add(v)
}

func (r *Repository) restore() error {
	dir, err := ioutil.ReadDir(r.configPath)
	if err != nil {
		return err
	}

	for _, v := range dir {
		id := v.Name()
		path, err := r.driver.Get(id, "")
		if err != nil {
			log.Debugf("Could not find volume for %s: %s", id, err)
			continue
		}
		name, err := r.generateNewName()
		if err != nil {
			log.Debugf("Error restoring volume %s: %s", id, err)
			continue
		}
		vol := &Volume{
			ID:         id,
			configPath: r.configPath + "/" + id,
			containers: make(map[string]struct{}),
			Path:       path,
			Name:       name,
		}
		if err := vol.FromDisk(); err != nil {
			if !os.IsNotExist(err) {
				log.Debugf("Error restoring volume: %s", err)
				continue
			}
			if err := vol.initialize(); err != nil {
				log.Debugf("%s", err)
			}
		}

		if err := r.add(vol); err != nil {
			log.Debugf("Error restoring volume: %s", err)
		}
	}
	return nil
}

func (r *Repository) Get(path string) *Volume {
	r.lock.Lock()
	vol := r.get(path)
	r.lock.Unlock()
	return vol
}

func (r *Repository) get(path string) *Volume {
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil
	}
	return r.volumes[path]
}

func (r *Repository) Add(volume *Volume) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.add(volume)
}

func (r *Repository) add(volume *Volume) error {
	if vol := r.get(volume.Path); vol != nil {
		return fmt.Errorf("Volume exists: %s", volume.ID)
	}

	r.volumes[volume.Path] = volume
	r.idIndex[volume.ID] = volume
	r.nameIndex[volume.Name] = volume
	return nil
}

func (r *Repository) Remove(volume *Volume) {
	r.lock.Lock()
	r.remove(volume)
	r.lock.Unlock()
}

func (r *Repository) remove(volume *Volume) {
	delete(r.volumes, volume.Path)
	delete(r.idIndex, volume.ID)
	delete(r.nameIndex, volume.Name)
}

func (r *Repository) Delete(name string) error {
	r.lock.Lock()

	volume := r.find(name)
	if volume == nil {
		r.lock.Unlock()
		return fmt.Errorf("Volume %s does not exist", name)
	}

	if volume.IsBindMount {
		r.lock.Unlock()
		return fmt.Errorf("Volume %s is a bind-mount and cannot be removed", volume.Name)
	}
	containers := volume.Containers()
	if len(containers) > 0 {
		r.lock.Unlock()
		return fmt.Errorf("Volume %s is being used and cannot be removed: used by containers %s", volume.Name, containers)
	}

	if err := os.RemoveAll(volume.configPath); err != nil {
		r.lock.Unlock()
		return err
	}
	r.remove(volume)

	r.lock.Unlock()

	// Run this outside the lock since it could take some time
	if err := r.driver.Remove(volume.ID); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

func (r *Repository) createNewVolumePath(id string) (string, error) {
	if err := r.driver.Create(id, ""); err != nil {
		return "", err
	}

	path, err := r.driver.Get(id, "")
	if err != nil {
		return "", fmt.Errorf("Driver %s failed to get volume rootfs %s: %s", r.driver, id, err)
	}

	return path, nil
}

func (r *Repository) FindOrCreateVolume(path, name string, writable bool) (*Volume, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if path == "" {
		return r.newVolume(path, name, writable)
	}

	if v := r.get(path); v != nil {
		return v, nil
	}

	return r.newVolume(path, name, writable)
}

func (r *Repository) NewVolume(path, name string, writable bool) (*Volume, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.newVolume(path, name, writable)
}

func (r *Repository) List() []*Volume {
	r.lock.Lock()
	var vols []*Volume

	for _, v := range r.volumes {
		vols = append(vols, v)
	}

	r.lock.Unlock()
	return vols
}

func (r *Repository) findByName(name string) *Volume {
	return r.nameIndex[name]
}

func (r *Repository) findByID(id string) *Volume {
	return r.idIndex[id]
}

func (r *Repository) find(name string) *Volume {
	if v := r.findByName(name); v != nil {
		return v
	}

	if v := r.findByID(name); v != nil {
		return v
	}

	return r.get(name)
}

func (r *Repository) Find(name string) *Volume {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.find(name)
}

func (r *Repository) generateNewName() (string, error) {
	for i := 0; i < 6; i++ {
		name := namesgenerator.GetRandomName(i)
		if v := r.findByName(name); v != nil {
			continue
		}

		return name, nil
	}

	return "", fmt.Errorf("Could not generate unique name")
}

func (r *Repository) generateNewId() (string, error) {
	for i := 0; i < 6; i++ {
		id := utils.GenerateRandomID()
		if v := r.findByID(id); v != nil || r.driver.Exists(id) {
			continue
		}

		return id, nil
	}
	return "", fmt.Errorf("Could not generate unique id")
}
