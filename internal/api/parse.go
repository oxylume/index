package api

import (
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sigurn/crc16"
)

var crc16table = crc16.MakeTable(crc16.CRC16_XMODEM)

func ParseAdnl(address string) ([]byte, error) {
	if len(address) != 55 {
		return nil, fmt.Errorf("wrong adnl address length")
	}

	decoded, err := base32.StdEncoding.DecodeString("F" + strings.ToUpper(address))
	if err != nil {
		return nil, fmt.Errorf("failed to decode address: %w", err)
	}

	if decoded[0] != 0x2d {
		return nil, fmt.Errorf("invalid adnl prefix")
	}

	expectedCrc := binary.BigEndian.Uint16(decoded[33:])
	calcCrc := crc16.Checksum(decoded[:33], crc16table)
	if expectedCrc != calcCrc {
		return nil, fmt.Errorf("invalid adnl address checksum")
	}

	return decoded[1:33], nil
}

func ParseRange(r *http.Request, max uint64) (from uint64, to uint64, hasRange bool, err error) {
	rangeHeader := r.Header.Get("Range")
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, max, false, nil
	}
	rangeHeader = rangeHeader[len("bytes="):]
	if strings.Contains(rangeHeader, ",") {
		return 0, 0, false, fmt.Errorf("multiple ranges not supported")
	}
	fromRange, toRange, found := strings.Cut(rangeHeader, "-")
	if !found {
		return 0, 0, false, fmt.Errorf("invalid range format")
	}
	if fromRange != "" {
		from, err = strconv.ParseUint(fromRange, 10, 64)
		if err != nil {
			return 0, 0, false, err
		}
		if from > max {
			return 0, 0, false, fmt.Errorf("from exceeds content length")
		}
	}
	if toRange != "" {
		to, err = strconv.ParseUint(toRange, 10, 64)
		if err != nil {
			return 0, 0, false, err
		}
		if to > max {
			return 0, 0, false, fmt.Errorf("to exceeds content length")
		}
	} else {
		to = max
	}

	if from > to {
		return 0, 0, false, fmt.Errorf("from cannot be higher than to")
	}
	return from, to, true, nil
}
