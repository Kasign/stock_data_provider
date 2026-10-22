package baostock

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEncodeFrameCountsUTF8BytesAndValidatesCRC(t *testing.T) {
	body := "login\x01anonymous\x01测试\x010"
	frame, err := encodeFrame(messageLoginRequest, body)
	if err != nil {
		t.Fatal(err)
	}
	messageType, bodyLength, err := parseHeader(frame[:21])
	if err != nil {
		t.Fatal(err)
	}
	if messageType != messageLoginRequest {
		t.Fatalf("message type=%q", messageType)
	}
	if bodyLength != len([]byte(body)) {
		t.Fatalf("body length=%d, want UTF-8 byte length %d", bodyLength, len([]byte(body)))
	}
	separator := bytes.LastIndexByte(frame, 0x01)
	if separator < 21+bodyLength {
		t.Fatalf("invalid frame separator position %d", separator)
	}
	if err := verifyCRC(frame[:21], frame[21:21+bodyLength], string(frame[separator+1:])); err != nil {
		t.Fatalf("encoded frame CRC is invalid: %v", err)
	}
}

func TestQueryDailyKLinesHandlesLoginCompressedQueryAndLogout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			requestFrame, err := reader.ReadBytes('\n')
			if err != nil {
				serverErr <- err
				return
			}
			requestFrame = bytes.TrimSuffix(requestFrame, []byte{'\n'})
			messageType, body, err := parseRequestForTest(requestFrame)
			if err != nil {
				serverErr <- err
				return
			}
			switch messageType {
			case messageLoginRequest:
				if !strings.HasPrefix(body, "login\x01anonymous\x01123456\x010") {
					serverErr <- fmt.Errorf("unexpected login body %q", body)
					return
				}
				if err := writeAll(conn, responseFrameForTest(messageLoginResponse, "0\x01success\x01login\x01server-session-987654", false)); err != nil {
					serverErr <- err
					return
				}
			case messageHistoryRequest:
				requestParts := strings.Split(body, messageSplit)
				if len(requestParts) != 10 {
					serverErr <- fmt.Errorf("query has %d fields, expected 10", len(requestParts))
					return
				}
				// The upstream Python client retains the configured session user ID.
				// The user_id echoed by login is result metadata, not a replacement
				// for the identity used in subsequent query and logout requests.
				if requestParts[1] != "anonymous" || requestParts[2] != "1" ||
					requestParts[4] != "sh.600519" || requestParts[8] != "d" || requestParts[9] != "2" {
					serverErr <- fmt.Errorf("unexpected query body %q", body)
					return
				}
				row := []string{"2026-10-21", "sh.600519", "1331.8800", "1331.9800", "1302.3600", "1309.6900", "1331.8800", "3351400", "4403823070", "2", "0.18", "1", "-1.66", "0"}
				payload, err := json.Marshal(map[string]any{"record": [][]string{row}})
				if err != nil {
					serverErr <- err
					return
				}
				responseBody := strings.Join([]string{
					"0", "success", "query_history_k_data_plus", "anonymous", "1", strconv.Itoa(defaultPageSize),
					string(payload), "sh.600519", dailyKLineFields, "2026-10-01", "2026-10-21", "d", "2",
				}, messageSplit)
				if err := writeAll(conn, responseFrameForTest(messageCompressed, responseBody, true)); err != nil {
					serverErr <- err
					return
				}
			case messageLogoutRequest:
				logoutParts := strings.Split(body, messageSplit)
				if len(logoutParts) != 3 || logoutParts[1] != "anonymous" {
					serverErr <- fmt.Errorf("logout must use configured session user ID, got %q", body)
					return
				}
				if err := writeAll(conn, responseFrameForTest(messageLogoutResponse, "0\x01success\x01logout", false)); err != nil {
					serverErr <- err
					return
				}
				serverErr <- nil
				return
			default:
				serverErr <- fmt.Errorf("unexpected message type %q", messageType)
				return
			}
		}
	}()

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(Config{Host: host, Port: port, ConnectTimeout: time.Second, RequestTimeout: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := client.QueryDailyKLines(ctx, "sh.600519", "2026-10-01", "2026-10-21", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Date != "2026-10-21" || rows[0].Volume != "3351400" || rows[0].Amount != "4403823070" {
		t.Fatalf("unexpected decoded row: %+v", rows[0])
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("test server failed: %v", err)
	}
}

func TestReadResponseRejectsBadCRC(t *testing.T) {
	frame := responseFrameForTest(messageLoginResponse, "0\x01success\x01login\x01anonymous", false)
	markerAt := bytes.Index(frame[21:], cdataTerminator) + 21
	if markerAt <= 21 {
		t.Fatal("test fixture has no CDATA terminator")
	}
	crcSeparator := bytes.LastIndex(frame[:markerAt], []byte{0x01})
	if crcSeparator < 0 {
		t.Fatal("test fixture has no CRC separator")
	}
	frame[crcSeparator+1] ^= 1
	_, _, err := readResponse(bufio.NewReader(bytes.NewReader(frame)))
	if err == nil || !strings.Contains(err.Error(), "CRC32") {
		t.Fatalf("expected CRC32 validation error, got %v", err)
	}
}

func parseRequestForTest(frame []byte) (string, string, error) {
	messageType, bodyLength, err := parseHeader(frame[:21])
	if err != nil {
		return "", "", err
	}
	if len(frame) < 21+bodyLength+2 {
		return "", "", fmt.Errorf("request frame too short: %d", len(frame))
	}
	body := frame[21 : 21+bodyLength]
	if frame[21+bodyLength] != 0x01 {
		return "", "", fmt.Errorf("request frame missing CRC separator")
	}
	if err := verifyCRC(frame[:21], body, string(frame[22+bodyLength:])); err != nil {
		return "", "", err
	}
	return messageType, string(body), nil
}

func responseFrameForTest(messageType, body string, compressed bool) []byte {
	bodyBytes := []byte(body)
	if compressed {
		var buffer bytes.Buffer
		writer := zlib.NewWriter(&buffer)
		_, _ = writer.Write(bodyBytes)
		_ = writer.Close()
		bodyBytes = buffer.Bytes()
	}
	header := []byte(fmt.Sprintf("%s%s%s%s%010d", clientProtocolVersion, messageSplit, messageType, messageSplit, len(bodyBytes)))
	headerAndBody := make([]byte, 0, len(header)+len(bodyBytes))
	headerAndBody = append(headerAndBody, header...)
	headerAndBody = append(headerAndBody, bodyBytes...)
	crc := strconv.FormatUint(uint64(crc32.ChecksumIEEE(headerAndBody)), 10)
	frame := append(headerAndBody, 0x01)
	frame = append(frame, crc...)
	if compressed {
		frame = append(frame, '\n')
	}
	frame = append(frame, cdataTerminator...)
	return frame
}
