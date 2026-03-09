//go:build linux

package dhcpd

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"

	"github.com/insomniacslk/dhcp/dhcpv4"

	"github.com/nadoo/glider/pkg/log"
)

func reply(iface *net.Interface, resp *dhcpv4.DHCPv4) error {
	p := [590]byte{12: 0x08, //ethernet layer: 14 bytes
		14: 0x45, 16: 0x02, 17: 0x40, 22: 0x40, 23: 0x11, //ip layer: 20 bytes
		35: 67, 37: 68, 38: 0x02, 39: 0x2c, //udp layer: 8 bytes
	}

	copy(p[0:], resp.ClientHWAddr[0:6])
	copy(p[6:], iface.HardwareAddr[0:6])
	copy(p[26:], resp.ServerIPAddr[0:4])
	copy(p[30:], resp.YourIPAddr[0:4])

	// IP 层校验和
	checksum := checksum(p[14:34])
	binary.BigEndian.PutUint16(p[24:], checksum)

	// DHCP 载荷
	copy(p[42:], resp.ToBytes())

	// UDP 层校验和，设为零
	// https://datatracker.ietf.org/doc/html/rfc768
	// 发送的校验和全为零表示发送方未生成校验和
	// （用于调试或不关心校验和的高层协议）。

	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, 0)
	if err != nil {
		return fmt.Errorf("cannot open socket: %v", err)
	}
	defer func() {
		err = syscall.Close(fd)
		if err != nil {
			log.F("dhcpd: cannot close socket: %v", err)
		}
	}()

	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		log.F("dhcpd: cannot set option for socket: %v", err)
	}

	var hwAddr [8]byte
	copy(hwAddr[0:6], resp.ClientHWAddr[0:6])
	ethAddr := syscall.SockaddrLinklayer{
		Protocol: 0,
		Ifindex:  iface.Index,
		Halen:    6,
		Addr:     hwAddr,
	}
	err = syscall.Sendto(fd, p[:], 0, &ethAddr)
	if err != nil {
		return fmt.Errorf("cannot send frame via socket: %v", err)
	}
	return nil
}

func checksum(bytes []byte) uint16 {
	var csum uint32
	for i := 0; i < len(bytes); i += 2 {
		csum += uint32(bytes[i]) << 8
		csum += uint32(bytes[i+1])
	}
	for {
		// 当累加和小于或等于 0xFFFF 时退出循环
		if csum <= 65535 {
			break
		}
		// 将进位加回到累加和中
		csum = (csum >> 16) + uint32(uint16(csum))
	}
	// 对所有位取反
	return ^uint16(csum)
}
