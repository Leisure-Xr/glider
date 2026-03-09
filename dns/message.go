package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math/rand/v2"
	"net/netip"
	"strings"
)

// UDPMaxLen 是 UDP DNS 请求的最大长度。
// https://www.dnsflagday.net/2020/
const UDPMaxLen = 1232

// HeaderLen 是 DNS 消息头的长度。
const HeaderLen = 12

// MsgType 是 DNS 消息类型。
type MsgType byte

// 消息类型。
const (
	QueryMsg    MsgType = 0
	ResponseMsg MsgType = 1
)

// 查询类型。
const (
	QTypeA    uint16 = 1  //ipv4
	QTypeAAAA uint16 = 28 ///ipv6
)

// ClassINET 互联网地址类。
const ClassINET uint16 = 1

// 消息格式：
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1
// 域名协议中的所有通信均以一种称为消息的统一格式传输。
// 消息的顶层格式分为 5 个部分（某些情况下部分为空），如下所示：
//
//	+---------------------+
//	|        Header       |
//	+---------------------+
//	|       Question      | 向名称服务器提出的查询
//	+---------------------+
//	|        Answer       | 回答查询的资源记录
//	+---------------------+
//	|      Authority      | 指向权威服务器的资源记录
//	+---------------------+
//	|      Additional     | 包含附加信息的资源记录
type Message struct {
	Header
	// 大多数 DNS 实现仅支持每次请求包含 1 个查询
	Question   *Question
	Answers    []*RR
	Authority  []*RR
	Additional []*RR

	// 用于 UnmarshalMessage
	unMarshaled []byte
}

// NewMessage 返回一个新的消息。
func NewMessage(id uint16, msgType MsgType) *Message {
	if id == 0 {
		id = uint16(rand.Uint32())
	}

	m := &Message{Header: Header{ID: id}}
	m.SetMsgType(msgType)

	return m
}

// SetQuestion 为 DNS 消息设置查询。
func (m *Message) SetQuestion(q *Question) error {
	m.Question = q
	m.Header.SetQdcount(1)
	return nil
}

// AddAnswer 向 DNS 消息中添加一条应答。
func (m *Message) AddAnswer(rr *RR) error {
	m.Answers = append(m.Answers, rr)
	return nil
}

// Marshal 将消息结构体序列化为 []byte。
func (m *Message) Marshal() ([]byte, error) {
	buf := &bytes.Buffer{}
	if _, err := m.MarshalTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalTo 将消息结构体序列化为 []byte 并写入 w。
func (m *Message) MarshalTo(w io.Writer) (n int, err error) {
	m.Header.SetQdcount(1)
	m.Header.SetAncount(len(m.Answers))

	nn := 0
	nn, err = m.Header.MarshalTo(w)
	if err != nil {
		return
	}
	n += nn

	nn, err = m.Question.MarshalTo(w)
	if err != nil {
		return
	}
	n += nn

	for _, answer := range m.Answers {
		nn, err = answer.MarshalTo(w)
		if err != nil {
			return
		}
		n += nn
	}

	return
}

// UnmarshalMessage 将 []byte 反序列化为 Message。
func UnmarshalMessage(b []byte) (*Message, error) {
	if len(b) < HeaderLen {
		return nil, errors.New("UnmarshalMessage: not enough data")
	}

	m := &Message{unMarshaled: b}
	if err := UnmarshalHeader(b[:HeaderLen], &m.Header); err != nil {
		return nil, err
	}

	q := &Question{}
	qLen, err := m.UnmarshalQuestion(b[HeaderLen:], q)
	if err != nil {
		return nil, err
	}
	m.SetQuestion(q)

	// 响应应答记录
	rrIdx := HeaderLen + qLen
	for range int(m.Header.ANCOUNT) {
		rr := &RR{}
		rrLen, err := m.UnmarshalRR(rrIdx, rr)
		if err != nil {
			return nil, err
		}
		m.AddAnswer(rr)

		rrIdx += rrLen
	}

	m.Header.SetAncount(len(m.Answers))

	return m, nil
}

// 消息头格式：
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.1
// 消息头包含以下字段：
//
//	                                1  1  1  1  1  1
//	  0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                      ID                       |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|QR|   Opcode  |AA|TC|RD|RA|   Z    |   RCODE   |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                    QDCOUNT                    |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                    ANCOUNT                    |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                    NSCOUNT                    |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                    ARCOUNT                    |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
type Header struct {
	ID      uint16
	Bits    uint16
	QDCOUNT uint16
	ANCOUNT uint16
	NSCOUNT uint16
	ARCOUNT uint16
}

// SetMsgType 设置消息类型。
func (h *Header) SetMsgType(qr MsgType) {
	h.Bits |= uint16(qr) << 15
}

// SetTC 设置 TC 标志位。
func (h *Header) SetTC(tc int) {
	h.Bits |= uint16(tc) << 9
}

// SetQdcount 设置查询数量，大多数 DNS 服务器每次请求仅支持 1 个查询。
func (h *Header) SetQdcount(qdcount int) {
	h.QDCOUNT = uint16(qdcount)
}

// SetAncount 设置应答数量。
func (h *Header) SetAncount(ancount int) {
	h.ANCOUNT = uint16(ancount)
}

// 暂未使用，保留以备将来使用。
// func (h *Header) setFlag(QR uint16, Opcode uint16, AA uint16,
// 	TC uint16, RD uint16, RA uint16, RCODE uint16) {
// 	h.Bits = QR<<15 + Opcode<<11 + AA<<10 + TC<<9 + RD<<8 + RA<<7 + RCODE
// }

// MarshalTo 将消息头结构体序列化为 []byte 并写入 w。
func (h *Header) MarshalTo(w io.Writer) (int, error) {
	return HeaderLen, binary.Write(w, binary.BigEndian, h)
}

// UnmarshalHeader 将 []byte 反序列化为 Header。
func UnmarshalHeader(b []byte, h *Header) error {
	if h == nil {
		return errors.New("unmarshal header must not be nil")
	}

	if len(b) != HeaderLen {
		return errors.New("unmarshal header bytes has an unexpected size")
	}

	h.ID = binary.BigEndian.Uint16(b[:2])
	h.Bits = binary.BigEndian.Uint16(b[2:4])
	h.QDCOUNT = binary.BigEndian.Uint16(b[4:6])
	h.ANCOUNT = binary.BigEndian.Uint16(b[6:8])
	h.NSCOUNT = binary.BigEndian.Uint16(b[8:10])
	h.ARCOUNT = binary.BigEndian.Uint16(b[10:])

	return nil
}

// 查询格式：
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.2
// 查询部分用于在大多数查询中携带"问题"，
// 即定义所查询内容的参数。该部分包含 QDCOUNT（通常为 1）条目，
// 每条格式如下：
//
//	                                1  1  1  1  1  1
//	  0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                                               |
//	/                     QNAME                     /
//	/                                               /
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                     QTYPE                     |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                     QCLASS                    |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
type Question struct {
	QNAME  string
	QTYPE  uint16
	QCLASS uint16
}

// NewQuestion 返回一个新的 DNS 查询。
func NewQuestion(qtype uint16, domain string) *Question {
	return &Question{
		QNAME:  domain,
		QTYPE:  qtype,
		QCLASS: ClassINET,
	}
}

// MarshalTo 将 Question 结构体序列化为 []byte 并写入 w。
func (q *Question) MarshalTo(w io.Writer) (n int, err error) {
	n, err = MarshalDomainTo(w, q.QNAME)
	if err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, q.QTYPE); err != nil {
		return
	}
	n += 2

	if err = binary.Write(w, binary.BigEndian, q.QCLASS); err != nil {
		return
	}
	n += 2

	return
}

// UnmarshalQuestion 将 []byte 反序列化为 Question。
func (m *Message) UnmarshalQuestion(b []byte, q *Question) (n int, err error) {
	if q == nil {
		return 0, errors.New("unmarshal question must not be nil")
	}

	if len(b) <= 5 {
		return 0, errors.New("UnmarshalQuestion: not enough data")
	}

	sb := new(strings.Builder)
	sb.Grow(32)
	idx, err := m.UnmarshalDomainTo(sb, b)
	if err != nil {
		return 0, err
	}

	q.QNAME = sb.String()
	q.QTYPE = binary.BigEndian.Uint16(b[idx : idx+2])
	q.QCLASS = binary.BigEndian.Uint16(b[idx+2 : idx+4])

	return idx + 3 + 1, nil
}

// 资源记录格式：
// https://www.rfc-editor.org/rfc/rfc1035#section-3.2.1
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.3
// 应答、权威和附加部分均共享相同格式：数量可变的资源记录，
// 记录数量由消息头中对应的计数字段指定。
// 每条资源记录格式如下：
//
//	                                1  1  1  1  1  1
//	  0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                                               |
//	/                                               /
//	/                      NAME                     /
//	|                                               |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                      TYPE                     |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                     CLASS                     |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                      TTL                      |
//	|                                               |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|                   RDLENGTH                    |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--|
//	/                     RDATA                     /
//	/                                               /
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
type RR struct {
	NAME     string
	TYPE     uint16
	CLASS    uint16
	TTL      uint32
	RDLENGTH uint16
	RDATA    []byte

	IP netip.Addr
}

// NewRR 返回一条新的 DNS 资源记录。
func NewRR() *RR {
	return &RR{}
}

// MarshalTo 将 RR 结构体序列化为 []byte 并写入 w。
func (rr *RR) MarshalTo(w io.Writer) (n int, err error) {
	n, err = MarshalDomainTo(w, rr.NAME)
	if err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, rr.TYPE); err != nil {
		return
	}
	n += 2

	if err = binary.Write(w, binary.BigEndian, rr.CLASS); err != nil {
		return
	}
	n += 2

	if err = binary.Write(w, binary.BigEndian, rr.TTL); err != nil {
		return
	}
	n += 4

	err = binary.Write(w, binary.BigEndian, rr.RDLENGTH)
	if err != nil {
		return
	}
	n += 2

	if _, err = w.Write(rr.RDATA); err != nil {
		return
	}
	n += len(rr.RDATA)

	return
}

// UnmarshalRR 将 []byte 反序列化为 RR。
func (m *Message) UnmarshalRR(start int, rr *RR) (n int, err error) {
	if rr == nil {
		return 0, errors.New("unmarshal rr must not be nil")
	}

	p := m.unMarshaled[start:]

	sb := new(strings.Builder)
	sb.Grow(32)

	n, err = m.UnmarshalDomainTo(sb, p)
	if err != nil {
		return 0, err
	}
	rr.NAME = sb.String()

	if len(p) <= n+10 {
		return 0, errors.New("UnmarshalRR: not enough data")
	}

	rr.TYPE = binary.BigEndian.Uint16(p[n:])
	rr.CLASS = binary.BigEndian.Uint16(p[n+2:])
	rr.TTL = binary.BigEndian.Uint32(p[n+4:])
	rr.RDLENGTH = binary.BigEndian.Uint16(p[n+8:])

	if len(p) < n+10+int(rr.RDLENGTH) {
		return 0, errors.New("UnmarshalRR: not enough data for RDATA")
	}

	rr.RDATA = p[n+10 : n+10+int(rr.RDLENGTH)]

	if rr.TYPE == QTypeA {
		rr.IP = netip.AddrFrom4(*(*[4]byte)(rr.RDATA[:4]))
	} else if rr.TYPE == QTypeAAAA {
		rr.IP = netip.AddrFrom16(*(*[16]byte)(rr.RDATA[:16]))
	}

	n = n + 10 + int(rr.RDLENGTH)

	return n, nil
}

// MarshalDomainTo 将域名字符串序列化为 []byte 并写入 w。
func MarshalDomainTo(w io.Writer, domain string) (n int, err error) {
	nn := 0
	for _, seg := range strings.Split(domain, ".") {
		nn, err = w.Write([]byte{byte(len(seg))})
		if err != nil {
			return
		}
		n += nn

		nn, err = io.WriteString(w, seg)
		if err != nil {
			return
		}
		n += nn
	}

	nn, err = w.Write([]byte{0x00})
	if err != nil {
		return
	}
	n += nn

	return
}

// UnmarshalDomainTo 从字节中解析域名并写入字符串构建器。
func (m *Message) UnmarshalDomainTo(sb *strings.Builder, b []byte) (int, error) {
	var idx, size int

	for len(b[idx:]) != 0 {
		// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.4
		// "消息压缩"，
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
		// | 1  1|                OFFSET                   |
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
		if b[idx]&0xC0 == 0xC0 {
			if len(b[idx:]) < 2 {
				return 0, errors.New("UnmarshalDomainTo: not enough size for compressed domain")
			}

			offset := binary.BigEndian.Uint16(b[idx : idx+2])
			if err := m.UnmarshalDomainPointTo(sb, int(offset&0x3FFF)); err != nil {
				return 0, err
			}

			idx += 2
			break
		}

		size = int(b[idx])
		idx++

		// 根域名
		if size == 0 {
			break
		}

		if size > 63 {
			return 0, errors.New("UnmarshalDomainTo: label size larger than 63")
		}

		if idx+size > len(b) {
			return 0, errors.New("UnmarshalDomainTo: label size larger than msg length")
		}

		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.Write(b[idx : idx+size])

		idx += size
	}

	return idx, nil
}

// UnmarshalDomainPointTo 从偏移量指向的位置解析域名并写入字符串构建器。
func (m *Message) UnmarshalDomainPointTo(sb *strings.Builder, offset int) error {
	if offset > len(m.unMarshaled) {
		return errors.New("UnmarshalDomainPointTo: offset larger than msg length")
	}
	_, err := m.UnmarshalDomainTo(sb, m.unMarshaled[offset:])
	return err
}
