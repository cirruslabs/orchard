package dhcpleasetime

import (
	"errors"
	"fmt"
	"howett.net/plist"
	"os"
)

const (
	internetSharingPath = "/Library/Preferences/SystemConfiguration/com.apple.InternetSharing.default.plist"
)

type InternetSharing struct {
	Bootpd Bootpd `plist:"bootpd"`
}

type Bootpd struct {
	DHCPLeaseTimeSecs int `plist:"DHCPLeaseTimeSecs"`
}

func Check() error {
	errDefault := fmt.Errorf("it seems that you're using a default DHCP lease time of 1 day which may result in " +
		"failures communicating with the VMs, please read https://tart.run/faq/#changing-the-default-dhcp-lease-time " +
		"for more details and intstructions on how to fix that")

	plistBytes, err := os.ReadFile(internetSharingPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errDefault
		}

		return fmt.Errorf("failed to check the default DHCP lease time: %w, please read "+
			"https://tart.run/faq/#changing-the-default-dhcp-lease-time for more details", err)
	}

	var internetSharing InternetSharing

	_, err = plist.Unmarshal(plistBytes, &internetSharing)
	if err != nil {
		return fmt.Errorf("failed to check the default DHCP lease time: %w, please read "+
			"https://tart.run/faq/#changing-the-default-dhcp-lease-time for more details", err)
	}

	if internetSharing.Bootpd.DHCPLeaseTimeSecs == 0 {
		return errDefault
	}

	if internetSharing.Bootpd.DHCPLeaseTimeSecs > 3600 {
		return fmt.Errorf("it seems that you're using a DHCP lease time larger than 1 hour which may result in " +
			"failures communicating with the VMs, please read https://tart.run/faq/#changing-the-default-dhcp-lease-time " +
			"for more details and intstructions on how to fix that")
	}

	return nil
}
