package common

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSendEmailUsesStartTLSWhenSSLEnabledOnSubmissionPort(t *testing.T) {
	restore := withSMTPSettings(t)
	defer restore()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake smtp server: %v", err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	cert := mustSMTPTestCert(t)
	go serveStartTLSSMTP(listener, serverErr, cert)

	host, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener addr: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(rawPort, "%d", &port); err != nil {
		t.Fatalf("parse listener port: %v", err)
	}

	SMTPServer = host
	SMTPPort = port
	SMTPSSLEnabled = true
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "smtp-token"
	SMTPForceAuthLogin = false

	if err := SendEmail("验证码", "receiver@example.com", "<p>code</p>"); err != nil {
		t.Fatalf("SendEmail() should use STARTTLS instead of implicit TLS on non-465 port: %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fake smtp server did not finish")
	}
}

func withSMTPSettings(t *testing.T) func() {
	t.Helper()

	originalServer := SMTPServer
	originalPort := SMTPPort
	originalSSLEnabled := SMTPSSLEnabled
	originalForceAuthLogin := SMTPForceAuthLogin
	originalAccount := SMTPAccount
	originalFrom := SMTPFrom
	originalToken := SMTPToken
	originalSystemName := SystemName

	SystemName = "New API"

	return func() {
		SMTPServer = originalServer
		SMTPPort = originalPort
		SMTPSSLEnabled = originalSSLEnabled
		SMTPForceAuthLogin = originalForceAuthLogin
		SMTPAccount = originalAccount
		SMTPFrom = originalFrom
		SMTPToken = originalToken
		SystemName = originalSystemName
	}
}

func serveStartTLSSMTP(listener net.Listener, serverErr chan<- error, cert tls.Certificate) {
	conn, err := listener.Accept()
	if err != nil {
		serverErr <- fmt.Errorf("accept fake smtp connection: %w", err)
		return
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		serverErr <- fmt.Errorf("set fake smtp deadline: %w", err)
		return
	}

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	if err := writeSMTPLine(writer, "220 smtp.test ESMTP ready"); err != nil {
		serverErr <- err
		return
	}

	firstByte, err := reader.ReadByte()
	if err != nil {
		serverErr <- fmt.Errorf("read first smtp command byte: %w", err)
		return
	}
	if firstByte == 0x16 {
		serverErr <- fmt.Errorf("client used implicit TLS before SMTP greeting on STARTTLS submission port")
		return
	}

	firstLine, err := reader.ReadString('\n')
	if err != nil {
		serverErr <- fmt.Errorf("read first smtp command: %w", err)
		return
	}
	if command := string(firstByte) + firstLine; !strings.HasPrefix(command, "EHLO ") {
		serverErr <- fmt.Errorf("first smtp command = %q, want EHLO", strings.TrimSpace(command))
		return
	}

	if err := writeSMTPLine(writer, "250-smtp.test greets you"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(writer, "250-STARTTLS"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(writer, "250 AUTH PLAIN"); err != nil {
		serverErr <- err
		return
	}

	if err := expectSMTPCommand(reader, "STARTTLS"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(writer, "220 ready to start TLS"); err != nil {
		serverErr <- err
		return
	}

	tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
	if err := tlsConn.Handshake(); err != nil {
		serverErr <- fmt.Errorf("server tls handshake: %w", err)
		return
	}
	defer tlsConn.Close()

	tlsReader := bufio.NewReader(tlsConn)
	tlsWriter := bufio.NewWriter(tlsConn)

	if err := expectSMTPCommand(tlsReader, "EHLO"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(tlsWriter, "250-smtp.test greets you"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(tlsWriter, "250 AUTH PLAIN"); err != nil {
		serverErr <- err
		return
	}
	if err := expectSMTPCommand(tlsReader, "AUTH PLAIN"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(tlsWriter, "235 authenticated"); err != nil {
		serverErr <- err
		return
	}
	if err := expectSMTPCommand(tlsReader, "MAIL FROM:"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(tlsWriter, "250 sender ok"); err != nil {
		serverErr <- err
		return
	}
	if err := expectSMTPCommand(tlsReader, "RCPT TO:"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(tlsWriter, "250 recipient ok"); err != nil {
		serverErr <- err
		return
	}
	if err := expectSMTPCommand(tlsReader, "DATA"); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(tlsWriter, "354 end data with <CR><LF>.<CR><LF>"); err != nil {
		serverErr <- err
		return
	}
	if err := readSMTPData(tlsReader); err != nil {
		serverErr <- err
		return
	}
	if err := writeSMTPLine(tlsWriter, "250 queued"); err != nil {
		serverErr <- err
		return
	}

	serverErr <- nil
}

func writeSMTPLine(writer *bufio.Writer, line string) error {
	if _, err := fmt.Fprintf(writer, "%s\r\n", line); err != nil {
		return fmt.Errorf("write smtp line %q: %w", line, err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush smtp line %q: %w", line, err)
	}
	return nil
}

func expectSMTPCommand(reader *bufio.Reader, prefix string) error {
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read smtp command %q: %w", prefix, err)
	}
	if !strings.HasPrefix(line, prefix) {
		return fmt.Errorf("smtp command = %q, want prefix %q", strings.TrimSpace(line), prefix)
	}
	return nil
}

func readSMTPData(reader *bufio.Reader) error {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("smtp data ended before terminator")
			}
			return fmt.Errorf("read smtp data: %w", err)
		}
		if line == ".\r\n" {
			return nil
		}
	}
}

func mustSMTPTestCert(t *testing.T) tls.Certificate {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate smtp test key: %v", err)
	}

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate smtp test cert serial: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create smtp test cert: %v", err)
	}

	keyDER := x509.MarshalPKCS1PrivateKey(privateKey)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("load smtp test cert: %v", err)
	}
	return cert
}
