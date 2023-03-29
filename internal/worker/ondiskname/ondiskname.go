package ondiskname

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotManagedByOrchard = errors.New("this on-disk VM is not managed by Orchard")
	ErrInvalidOnDiskName   = errors.New("invalid on-disk VM name")
)

const (
	prefix           = "orchard"
	numHyphensInUUID = 5
)

type OnDiskName struct {
	Name string
	UID  string
}

func New(name string, uid string) OnDiskName {
	return OnDiskName{
		Name: name,
		UID:  uid,
	}
}

func Parse(s string) (OnDiskName, error) {
	splits := strings.Split(s, "-")

	if !strings.HasPrefix(s, fmt.Sprintf("%s-", prefix)) {
		return OnDiskName{}, ErrNotManagedByOrchard
	}

	if len(splits) < 7 {
		return OnDiskName{}, fmt.Errorf("%w: name should contain at least 7 parts delimited by \"-\"",
			ErrInvalidOnDiskName)
	}

	for _, split := range splits {
		if split == "" {
			return OnDiskName{}, fmt.Errorf("%w: name should not contain empty parts",
				ErrInvalidOnDiskName)
		}
	}

	if splits[0] != prefix {
		return OnDiskName{}, fmt.Errorf("%w: name should begin with \"%s\" prefix",
			ErrInvalidOnDiskName, prefix)
	}

	uuidStart := len(splits) - numHyphensInUUID

	return OnDiskName{
		Name: strings.Join(splits[1:uuidStart], "-"),
		UID:  strings.Join(splits[uuidStart:], "-"),
	}, nil
}

func (odn OnDiskName) String() string {
	return fmt.Sprintf("%s-%s-%s", prefix, odn.Name, odn.UID)
}
