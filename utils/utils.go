package utils

import (
	"net"
)

func ResolveSourceIPAddressFromRawPacket(packet []byte) string {
	return net.IPv4(packet[12], packet[13], packet[14], packet[15]).To4().String()
}

func ResolveDestinationIPAddressFromRawPacket(packet []byte) string {
	return net.IPv4(packet[16], packet[17], packet[18], packet[19]).To4().String()
}


