package util

import (
	"fmt"
	"io"
	"net"
	"os"
)

// CopyFile copies a file from srcPath to dstPath.
// Returns an error if either file operation fails.
func CopyFile(srcPath, dstPath string) error {
	fin, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer fin.Close()

	fout, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer fout.Close()

	_, err = io.Copy(fout, fin)
	return err
}

// GetLocalIP returns the first non-loopback IPv4 address from available network interfaces.
// Returns an error if no suitable interface is found or if reading interfaces fails.
func GetLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no suitable interface found")
}
