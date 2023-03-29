package ondiskname

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidOnDiskName = errors.New("invalid on-disk VM name")

const prefix = "orchard"

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

	if len(splits) < 3 {
		return OnDiskName{}, fmt.Errorf("%w: name should contain at least 3 parts delimited by \"-\"",
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

	return OnDiskName{
		Name: splits[1],
		UID:  strings.Join(splits[2:], "-"),
	}, nil
}

func (odn OnDiskName) String() string {
	return fmt.Sprintf("%s-%s-%s", prefix, odn.Name, odn.UID)
}
