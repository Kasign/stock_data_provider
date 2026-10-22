package baostock

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strconv"
	"strings"
)

const (
	clientProtocolVersion = "00.9.10"
	maxFrameBytes         = 64 << 20
	messageSplit          = "\x01"
	messageLoginRequest   = "00"
	messageLoginResponse  = "01"
	messageLogoutRequest  = "02"
	messageLogoutResponse = "03"
	messageException      = "04"
	messageHistoryRequest = "95"
	messageCompressed     = "96"
)

var cdataTerminator = []byte("<![CDATA[]]>\n")

func encodeFrame(messageType, body string) ([]byte, error) {
	if len(messageType) != 2 {
		return nil, fmt.Errorf("BaoStock message type must contain two characters, got %q", messageType)
	}
	bodyBytes := []byte(body)
	if len(bodyBytes) > maxFrameBytes {
		return nil, fmt.Errorf("BaoStock request body exceeds %d bytes", maxFrameBytes)
	}

	header := []byte(fmt.Sprintf("%s%s%s%s%010d",
		clientProtocolVersion, messageSplit, messageType, messageSplit, len(bodyBytes)))
	if len(header) != 21 {
		return nil, fmt.Errorf("internal BaoStock header length is %d, expected 21", len(header))
	}

	headerAndBody := make([]byte, 0, len(header)+len(bodyBytes))
	headerAndBody = append(headerAndBody, header...)
	headerAndBody = append(headerAndBody, bodyBytes...)
	crc := strconv.FormatUint(uint64(crc32.ChecksumIEEE(headerAndBody)), 10)

	frame := make([]byte, 0, len(headerAndBody)+1+len(crc))
	frame = append(frame, headerAndBody...)
	frame = append(frame, 0x01)
	frame = append(frame, crc...)
	return frame, nil
}

func readResponse(reader *bufio.Reader) (string, string, error) {
	var header [21]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return "", "", err
	}
	messageType, bodyLength, err := parseHeader(header[:])
	if err != nil {
		return "", "", err
	}
	if bodyLength < 0 || bodyLength > maxFrameBytes {
		return "", "", fmt.Errorf("BaoStock response body length %d exceeds safety limit", bodyLength)
	}

	if messageType == messageCompressed {
		compressedBody := make([]byte, bodyLength)
		if bodyLength > 0 {
			if _, err := io.ReadFull(reader, compressedBody); err != nil {
				return "", "", err
			}
		}

		trailer, err := reader.ReadBytes('\n')
		if err != nil {
			return "", "", err
		}
		if len(trailer) < 3 || trailer[0] != 0x01 {
			return "", "", errors.New("compressed BaoStock response is missing CRC trailer")
		}
		if err := verifyCRC(header[:], compressedBody, strings.TrimSpace(string(trailer[1:]))); err != nil {
			return "", "", err
		}
		terminator := make([]byte, len(cdataTerminator))
		if _, err := io.ReadFull(reader, terminator); err != nil {
			return "", "", err
		}
		if !bytes.Equal(terminator, cdataTerminator) {
			return "", "", errors.New("compressed BaoStock response is missing CDATA terminator")
		}

		zr, err := zlib.NewReader(bytes.NewReader(compressedBody))
		if err != nil {
			return "", "", fmt.Errorf("open BaoStock zlib payload: %w", err)
		}
		defer zr.Close()
		plain, err := io.ReadAll(io.LimitReader(zr, maxFrameBytes+1))
		if err != nil {
			return "", "", fmt.Errorf("decompress BaoStock response: %w", err)
		}
		if len(plain) > maxFrameBytes {
			return "", "", fmt.Errorf("decompressed BaoStock response exceeds %d bytes", maxFrameBytes)
		}
		return messageType, string(plain), nil
	}

	// Non-compressed responses are delimited by CDATA; the advertised length
	// can be incorrect for UTF-8 error messages, so it is not used as a boundary.
	buffer := make([]byte, 0, 2048)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", "", err
		}
		buffer = append(buffer, b)
		if len(buffer) > maxFrameBytes {
			return "", "", fmt.Errorf("BaoStock response exceeds %d bytes", maxFrameBytes)
		}
		if bytes.HasSuffix(buffer, cdataTerminator) {
			buffer = buffer[:len(buffer)-len(cdataTerminator)]
			break
		}
	}
	separator := bytes.LastIndexByte(buffer, 0x01)
	if separator < 0 {
		return "", "", errors.New("BaoStock response is missing its CRC separator")
	}
	bodyBytes := buffer[:separator]
	crcText := strings.TrimSpace(string(buffer[separator+1:]))
	if err := verifyCRC(header[:], bodyBytes, crcText); err != nil {
		return "", "", err
	}
	return messageType, string(bodyBytes), nil
}

func parseHeader(header []byte) (messageType string, bodyLength int, err error) {
	if len(header) != 21 {
		return "", 0, fmt.Errorf("BaoStock header has %d bytes, expected 21", len(header))
	}
	parts := strings.Split(string(header), messageSplit)
	if len(parts) != 3 {
		return "", 0, errors.New("malformed BaoStock message header")
	}
	if parts[0] != clientProtocolVersion {
		return "", 0, fmt.Errorf("unsupported BaoStock protocol version %q (expected %q)", parts[0], clientProtocolVersion)
	}
	if len(parts[1]) != 2 {
		return "", 0, fmt.Errorf("invalid BaoStock message type %q", parts[1])
	}
	length, parseErr := strconv.Atoi(parts[2])
	if parseErr != nil || length < 0 {
		return "", 0, fmt.Errorf("invalid BaoStock body length %q", parts[2])
	}
	return parts[1], length, nil
}

func verifyCRC(header, body []byte, rawCRC string) error {
	expected, err := strconv.ParseUint(strings.TrimSpace(rawCRC), 10, 32)
	if err != nil {
		return fmt.Errorf("invalid BaoStock CRC32 %q", rawCRC)
	}
	content := make([]byte, 0, len(header)+len(body))
	content = append(content, header...)
	content = append(content, body...)
	actual := crc32.ChecksumIEEE(content)
	if uint64(actual) != expected {
		return fmt.Errorf("BaoStock CRC32 mismatch: expected %d, calculated %d", expected, actual)
	}
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
