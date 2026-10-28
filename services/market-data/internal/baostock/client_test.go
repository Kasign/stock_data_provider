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

func TestParseHeaderRejectsUnknownProtocolVersion(t *testing.T) {
	header := []byte(fmt.Sprintf("99.9.99%s%s%s%010d", messageSplit, messageLoginResponse, messageSplit, 0))
	if len(header) != 21 {
		t.Fatalf("test header has %d bytes, expected 21", len(header))
	}
	_, _, err := parseHeader(header)
	if err == nil || !strings.Contains(err.Error(), "unsupported BaoStock protocol version") {
		t.Fatalf("expected unknown protocol version error, got %v", err)
	}
}

func TestQueryTradeDatesHandlesLoginQueryAndLogout(t *testing.T) {
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
		if err := serveBaoStockLoginForTest(conn, reader); err != nil {
			serverErr <- err
			return
		}
		messageType, body, err := readBaoStockRequestForTest(reader)
		if err != nil {
			serverErr <- err
			return
		}
		parts := strings.Split(body, messageSplit)
		if messageType != messageTradeDateRequest || len(parts) != 6 ||
			parts[0] != "query_trade_dates" || parts[1] != "anonymous" ||
			parts[2] != "1" || parts[3] != strconv.Itoa(defaultPageSize) ||
			parts[4] != "2026-10-22" || parts[5] != "2026-10-24" {
			serverErr <- fmt.Errorf("unexpected trading-calendar request type=%q body=%q", messageType, body)
			return
		}
		raw, err := json.Marshal(map[string]any{
			"record": [][]string{
				{"2026-10-22", "1"},
				{"2026-10-23", "1"},
				{"2026-10-24", "0"},
			},
		})
		if err != nil {
			serverErr <- err
			return
		}
		responseBody := strings.Join([]string{
			"0", "success", "query_trade_dates", "anonymous", "1",
			strconv.Itoa(defaultPageSize), string(raw), "2026-10-22",
			"2026-10-24", "calendar_date,is_trading_day",
		}, messageSplit)
		if err := writeAll(conn, responseFrameForTest(messageTradeDateResponse, responseBody, false)); err != nil {
			serverErr <- err
			return
		}
		messageType, body, err = readBaoStockRequestForTest(reader)
		if err != nil {
			serverErr <- err
			return
		}
		if messageType != messageLogoutRequest || !strings.HasPrefix(body, "logout\x01anonymous\x01") {
			serverErr <- fmt.Errorf("expected logout after trading-calendar query, got type=%q body=%q", messageType, body)
			return
		}
		if err := writeAll(conn, responseFrameForTest(messageLogoutResponse, "0\x01success\x01logout", false)); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(Config{
		Host: host, Port: port, ConnectTimeout: time.Second, RequestTimeout: time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := client.QueryTradeDates(ctx, "2026-10-22", "2026-10-24")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Date != "2026-10-22" || !rows[0].IsTradingDay ||
		rows[1].Date != "2026-10-23" || !rows[1].IsTradingDay ||
		rows[2].Date != "2026-10-24" || rows[2].IsTradingDay {
		t.Fatalf("unexpected trading-calendar rows: %+v", rows)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("mock BaoStock server failed: %v", err)
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

func TestReadCompressedResponseAcceptsValidZlibWithMismatchedWireCRC(t *testing.T) {
	body := "0\x01success\x01query_history_k_data_plus\x01anonymous"
	frame := responseFrameForTest(messageCompressed, body, true)
	_, bodyLength, err := parseHeader(frame[:21])
	if err != nil {
		t.Fatal(err)
	}
	separator := 21 + bodyLength
	if separator >= len(frame) || frame[separator] != 0x01 {
		t.Fatal("compressed response has no CRC separator")
	}
	crcStart := separator + 1
	newlineOffset := bytes.IndexByte(frame[crcStart:], '\n')
	if newlineOffset <= 0 {
		t.Fatal("compressed response has no CRC trailer")
	}
	crcEnd := crcStart + newlineOffset
	if frame[crcEnd-1] == '0' {
		frame[crcEnd-1] = '1'
	} else {
		frame[crcEnd-1] = '0'
	}

	messageType, got, err := readResponse(bufio.NewReader(bytes.NewReader(frame)))
	if err != nil {
		t.Fatalf("valid zlib payload should survive non-matching wire CRC: %v", err)
	}
	if messageType != messageCompressed || got != body {
		t.Fatalf("unexpected response: type=%q body=%q", messageType, got)
	}
}

func TestReadCompressedResponseRejectsMalformedCRCText(t *testing.T) {
	frame := responseFrameForTest(messageCompressed, "0\x01success", true)
	_, bodyLength, err := parseHeader(frame[:21])
	if err != nil {
		t.Fatal(err)
	}
	separator := 21 + bodyLength
	if separator >= len(frame) || frame[separator] != 0x01 || separator+1 >= len(frame) {
		t.Fatal("compressed response has no CRC trailer")
	}
	frame[separator+1] = 'x'

	_, _, err = readResponse(bufio.NewReader(bytes.NewReader(frame)))
	if err == nil || !strings.Contains(err.Error(), "invalid BaoStock CRC32") {
		t.Fatalf("expected malformed CRC trailer error, got %v", err)
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

func readBaoStockRequestForTest(reader *bufio.Reader) (string, string, error) {
	frame, err := reader.ReadBytes('\n')
	if err != nil {
		return "", "", err
	}
	return parseRequestForTest(bytes.TrimSuffix(frame, []byte{'\n'}))
}

func serveBaoStockLoginForTest(conn net.Conn, reader *bufio.Reader) error {
	messageType, body, err := readBaoStockRequestForTest(reader)
	if err != nil {
		return err
	}
	if messageType != messageLoginRequest || !strings.HasPrefix(body, "login\x01anonymous\x01123456\x010") {
		return fmt.Errorf("unexpected login request type/body: %q %q", messageType, body)
	}
	return writeAll(conn, responseFrameForTest(messageLoginResponse, "0\x01success\x01login\x01server-session", false))
}

func klinePageResponseForTest(pageNumber, pageSize int, rows [][]string) []byte {
	payload, _ := json.Marshal(map[string]any{"record": rows})
	body := strings.Join([]string{
		"0", "success", "query_history_k_data_plus", "anonymous",
		strconv.Itoa(pageNumber), strconv.Itoa(pageSize), string(payload),
		"sh.600519", dailyKLineFields, "2026-10-01", "2026-10-21", "d", "2",
	}, messageSplit)
	return responseFrameForTest(messageCompressed, body, true)
}

func validTestKLineRowForDate(date string) []string {
	return []string{date, "sh.600519", "10.0", "10.2", "9.8", "10.1", "9.9",
		"1000", "10000", "2", "0.1", "1", "1.0", "0"}
}

func TestQueryDailyKLinesPaginatesAndAcceptsEmptyLastPage(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		if err := serveBaoStockLoginForTest(conn, reader); err != nil {
			serverErr <- err
			return
		}
		// 用较小的响应 pageSize 模拟分页边界，避免测试生成一万条重复样本。
		for pageNumber := 1; pageNumber <= 3; pageNumber++ {
			messageType, body, err := readBaoStockRequestForTest(reader)
			if err != nil {
				serverErr <- err
				return
			}
			parts := strings.Split(body, messageSplit)
			if messageType != messageHistoryRequest || len(parts) != 10 || parts[2] != strconv.Itoa(pageNumber) {
				serverErr <- fmt.Errorf("unexpected page request: type=%q body=%q", messageType, body)
				return
			}
			var rows [][]string
			switch pageNumber {
			case 1:
				rows = [][]string{validTestKLineRowForDate("2026-10-20")}
			case 2:
				rows = [][]string{validTestKLineRowForDate("2026-10-21")}
			case 3:
				// 空的最后一页应结束分页，但不能丢弃之前已收到的记录。
				rows = [][]string{}
			}
			if err := writeAll(conn, klinePageResponseForTest(pageNumber, 1, rows)); err != nil {
				serverErr <- err
				return
			}
		}
		messageType, _, err := readBaoStockRequestForTest(reader)
		if err != nil {
			serverErr <- err
			return
		}
		if messageType != messageLogoutRequest {
			serverErr <- fmt.Errorf("expected logout after completed query, got %q", messageType)
			return
		}
		if err := writeAll(conn, responseFrameForTest(messageLogoutResponse, "0\x01success\x01logout", false)); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	rows, err := client.QueryDailyKLines(ctx, "sh.600519", "2026-10-01", "2026-10-21", "2")
	if err != nil {
		t.Fatalf("分页查询失败：%v", err)
	}
	if len(rows) != 2 || rows[0].Date != "2026-10-20" || rows[1].Date != "2026-10-21" {
		t.Fatalf("分页结果丢失或顺序错误：%+v", rows)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("登出失败：%v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("模拟 BaoStock 服务端失败：%v", err)
	}
}

func TestQueryDailyKLinesReturnsUpstreamError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		if err := serveBaoStockLoginForTest(conn, reader); err != nil {
			serverErr <- err
			return
		}
		messageType, _, err := readBaoStockRequestForTest(reader)
		if err != nil {
			serverErr <- err
			return
		}
		if messageType != messageHistoryRequest {
			serverErr <- fmt.Errorf("expected history request, got %q", messageType)
			return
		}
		// 服务端业务错误走压缩响应通道，客户端必须保留错误码/错误信息。
		if err := writeAll(conn, responseFrameForTest(messageCompressed, "1\x01invalid date range", true)); err != nil {
			serverErr <- err
			return
		}
		messageType, _, err = readBaoStockRequestForTest(reader)
		if err != nil {
			serverErr <- err
			return
		}
		if messageType != messageLogoutRequest {
			serverErr <- fmt.Errorf("expected logout after provider error, got %q", messageType)
			return
		}
		if err := writeAll(conn, responseFrameForTest(messageLogoutResponse, "0\x01success\x01logout", false)); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
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

	_, err = client.QueryDailyKLines(ctx, "sh.600519", "2026-10-01", "2026-10-21", "2")
	if err == nil || !strings.Contains(err.Error(), "code=1") || !strings.Contains(err.Error(), "invalid date range") {
		t.Fatalf("应返回上游错误码与原因，实际错误：%v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("上游错误后登出失败：%v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("模拟 BaoStock 服务端失败：%v", err)
	}
}

func TestQueryDailyKLinesReconnectsAfterConnectionDrops(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		// 第一个会话在收到查询后断开连接，模拟上游重置长连接。
		first, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		firstReader := bufio.NewReader(first)
		if err := serveBaoStockLoginForTest(first, firstReader); err != nil {
			_ = first.Close()
			serverErr <- err
			return
		}
		messageType, _, err := readBaoStockRequestForTest(firstReader)
		if err != nil || messageType != messageHistoryRequest {
			_ = first.Close()
			serverErr <- fmt.Errorf("expected first history request, type=%q err=%v", messageType, err)
			return
		}
		_ = first.Close()

		// 后续调用必须新建 TCP 连接、重新登录，再继续读取日 K。
		second, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		defer second.Close()
		secondReader := bufio.NewReader(second)
		if err := serveBaoStockLoginForTest(second, secondReader); err != nil {
			serverErr <- err
			return
		}
		messageType, _, err = readBaoStockRequestForTest(secondReader)
		if err != nil || messageType != messageHistoryRequest {
			serverErr <- fmt.Errorf("expected second history request, type=%q err=%v", messageType, err)
			return
		}
		if err := writeAll(second, klinePageResponseForTest(1, defaultPageSize, [][]string{validTestKLineRowForDate("2026-10-21")})); err != nil {
			serverErr <- err
			return
		}
		messageType, _, err = readBaoStockRequestForTest(secondReader)
		if err != nil || messageType != messageLogoutRequest {
			serverErr <- fmt.Errorf("expected logout after reconnected query, type=%q err=%v", messageType, err)
			return
		}
		if err := writeAll(second, responseFrameForTest(messageLogoutResponse, "0\x01success\x01logout", false)); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	if _, err := client.QueryDailyKLines(ctx, "sh.600519", "2026-10-01", "2026-10-21", "2"); err == nil {
		t.Fatal("断线的首次查询应明确返回错误，不能伪造成功数据")
	}
	rows, err := client.QueryDailyKLines(ctx, "sh.600519", "2026-10-01", "2026-10-21", "2")
	if err != nil {
		t.Fatalf("断线后的下一次调用应重新连接并成功查询：%v", err)
	}
	if len(rows) != 1 || rows[0].Date != "2026-10-21" {
		t.Fatalf("重连后的结果异常：%+v", rows)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("重连后的登出失败：%v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("模拟 BaoStock 服务端失败：%v", err)
	}
}

func TestQueryQuarterlyFinancialDataHandlesLoginQueryAndLogout(t *testing.T) {
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
		if err := serveBaoStockLoginForTest(conn, reader); err != nil {
			serverErr <- err
			return
		}
		messageType, body, err := readBaoStockRequestForTest(reader)
		if err != nil {
			serverErr <- err
			return
		}
		parts := strings.Split(body, messageSplit)
		if messageType != messageProfitRequest || len(parts) != 7 ||
			parts[0] != "query_profit_data" || parts[1] != "anonymous" || parts[2] != "1" ||
			parts[3] != strconv.Itoa(quarterlyFinancialPageSize) || parts[4] != "sh.600519" || parts[5] != "2026" || parts[6] != "2" {
			serverErr <- fmt.Errorf("unexpected financial request type=%q body=%q", messageType, body)
			return
		}
		row := []string{"sh.600519", "2026-08-29", "2026-06-30", "0.074617", "0.342179", "0.25", "28522000000.000000", "1.939029", "83354000000.000000", "28103763899.00", "28103763899.00"}
		raw, err := json.Marshal(map[string]any{"record": [][]string{row}})
		if err != nil {
			serverErr <- err
			return
		}
		fields := strings.Join(quarterlyFinancialSpecs["profitability"].fields, ",")
		body = strings.Join([]string{"0", "success", "query_profit_data", "anonymous", "1", strconv.Itoa(quarterlyFinancialPageSize), string(raw), "sh.600519", "2026", "2", fields}, messageSplit)
		if err := writeAll(conn, responseFrameForTest(messageProfitResponse, body, false)); err != nil {
			serverErr <- err
			return
		}
		messageType, body, err = readBaoStockRequestForTest(reader)
		if err != nil {
			serverErr <- err
			return
		}
		if messageType != messageLogoutRequest || !strings.HasPrefix(body, "logout\x01anonymous\x01") {
			serverErr <- fmt.Errorf("expected logout; got %q %q", messageType, body)
			return
		}
		if err := writeAll(conn, responseFrameForTest(messageLogoutResponse, "0\x01success\x01logout", false)); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
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
	record, err := client.QueryQuarterlyFinancialData(ctx, "sh.600519", "profitability", 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || record.Code != "sh.600519" || record.ReportDate != "2026-06-30" || record.PublicationDate != "2026-08-29" {
		t.Fatalf("unexpected financial row: %+v", record)
	}
	if record.Fields["netProfit"] != "28522000000.000000" || record.Fields["roeAvg"] != "0.074617" {
		t.Fatalf("raw values changed: %+v", record.Fields)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("mock BaoStock server failed: %v", err)
	}
}

func TestQueryQuarterlyFinancialDataRejectsFutureReportingPeriodBeforeNetworkIO(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil { loc = time.FixedZone("CST", 8*60*60) }
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")
	quarter := int((int(now.Month())-1)/3) + 1
	periodEnd := time.Date(now.Year(), time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	if periodEnd <= today { quarter++ }
	if quarter > 4 { t.Skip("no future quarter end remains in the current year") }

	c := NewClient(Config{})
	if _, err := c.QueryQuarterlyFinancialData(context.Background(), "sh.600519", "profitability", now.Year(), quarter); err == nil || !strings.Contains(err.Error(), "is in the future") {
		t.Fatalf("future quarterly reporting period should be rejected before network IO, got %v", err)
	}
}

func TestQueryQuarterlyFinancialDataRejectsInvalidInputsBeforeNetworkIO(t *testing.T) {
	c := NewClient(Config{})
	if _, err := c.QueryQuarterlyFinancialData(context.Background(), "sh.600519", "missing", 2026, 2); err == nil {
		t.Fatal("unsupported category should fail")
	}
	if _, err := c.QueryQuarterlyFinancialData(context.Background(), "sh.600519", "profitability", 2026, 5); err == nil {
		t.Fatal("invalid quarter should fail")
	}
	if _, err := c.QueryQuarterlyFinancialData(context.Background(), "600519.SH", "profitability", 2026, 2); err == nil {
		t.Fatal("wire-format symbol should be required")
	}
}
