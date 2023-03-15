package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

const WebSocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func main() {
	http.HandleFunc("/connect", Chat)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}

func Chat(resp http.ResponseWriter, req *http.Request) {
	_, rw, err := Shank(resp, req)
	if err != nil {
		resp.WriteHeader(http.StatusForbidden)
		return
	}
	reader := rw.Reader
	writer := rw.Writer
	Write("连接成功,准备开始喝水提醒，默认60分钟提醒一次", writer)
	var ticker *time.Ticker
	minute := int64(60)
	ticker = time.NewTicker(time.Minute * time.Duration(minute))
	for {
		go func() {
			select {
			case <-ticker.C:
				Write(fmt.Sprintf("过去%d分钟啦，注意喝水!", minute), writer)
			}
		}()
		userMessage := Read(reader)
		var err error
		minute, err = strconv.ParseInt(userMessage, 10, 64)
		if err != nil {
			Write("输入指令错误，请输入整数调整提醒间隔（单位：分钟）", writer)
		} else {
			Write(fmt.Sprintf("重置成功，提醒间隔已重置为%d分钟。", minute), writer)
			ticker.Reset(time.Minute * time.Duration(minute))
		}
	}
}

func Read(rd *bufio.Reader) string {
	f := Frame{}
	f.Decode(rd)

	data := string(f.data)
	fmt.Println("read message,", data, ",frame:%v", f)
	return data
}

func Write(serverData string, writer *bufio.Writer) {
	frame := Frame{
		Fin:        true,
		Rsv:        [3]bool{},
		OpCode:     0x1,
		Mask:       false,
		Length:     int64(len(serverData)),
		MaskingKey: []byte{},
		data:       []byte(serverData),
	}
	data, err := frame.Encode()
	if err != nil {
		fmt.Println("frame encode err,", err)
		//163 180
	}
	nn, err := writer.Write(data)
	if err != nil {
		fmt.Println("write err,", err, "nn,", nn)
	}
	err = writer.Flush()
	if err != nil {
		fmt.Println("write flush err,", err)
	}
	fmt.Println("write data success")
}

// websocket握手
func Shank(resp http.ResponseWriter, req *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	fmt.Println("connect begin")
	secKet := req.Header.Get("Sec-WebSocket-Key")
	//未升级到websocket协议
	if req.Header.Get("Connection") != "Upgrade" ||
		req.Header.Get("Upgrade") != "websocket" ||
		secKet == "" {
		fmt.Printf("upgrade error,connetion:%v,upgrade:%v,websocket:%v\n", req.Header.Get("Connection"), req.Header.Get("Upgrade"), secKet)
		return nil, nil, errors.New("upgrade err")
	}
	//使用http.hijacker接管http连接，否则连接会在返回后进行释放，就无法进行持续的websocket通信了
	jk, ok := resp.(http.Hijacker)
	if !ok {
		fmt.Println("hijack conv err")
		//正常返回，无法建立websocket连接
		return nil, nil, errors.New("hijack conv err")
	}
	conn, buf, err := jk.Hijack()
	if err != nil {
		fmt.Println("hijack err")
		//正常返回，无法建立websocket连接
		return nil, nil, errors.New("hijack err")
	}
	acceptSecKey := make([]byte, 28)
	hash := sha1.New()
	hash.Write([]byte(secKet))
	hash.Write([]byte(WebSocketGUID))
	base64.StdEncoding.Encode(acceptSecKey, hash.Sum(nil))
	writer := buf.Writer
	writer.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	writer.WriteString("Connection: Upgrade\r\n")
	writer.WriteString("Upgrade: websocket\r\n")
	writer.WriteString("Sec-WebSocket-Accept: " + string(acceptSecKey) + "\r\n")
	writer.WriteString("\r\n")
	writer.Flush()
	fmt.Println("write resp success")
	return conn, buf, nil
}

// 0                   1                   2                   3
//
//	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//	+-+-+-+-+-------+-+-------------+-------------------------------+
//	|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
//	|I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
//	|N|V|V|V|       |S|             |   (if payload len==126/127)   |
//	| |1|2|3|       |K|             |                               |
//	+-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
//	|     Extended payload length continued, if payload len == 127  |
//	+ - - - - - - - - - - - - - - - +-------------------------------+
//	|                               |Masking-key, if MASK set to 1  |
//	+-------------------------------+-------------------------------+
//	| Masking-key (continued)       |          Payload Data         |
//	+-------------------------------- - - - - - - - - - - - - - - - +
//	:                     Payload Data continued ...                :
//	+ - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
//	|                     Payload Data continued ...                |
//	+---------------------------------------------------------------+

type Frame struct {
	Fin bool //　　FIN:1位，用于描述消息是否结束，如果为1则该消息为消息尾部,如果为零则还有后续数据包;

	Rsv [3]bool //　　RSV1,RSV2,RSV3：各1位，用于扩展定义的,如果没有扩展约定的情况则必须为0

	OpCode byte //4位,如果接收到未知的opcode，接收端必须关闭连接
	//　　OPCODE定义的范围：
	//
	//　　　　0x0表示附加数据帧
	//　　　　0x1表示文本数据帧
	//　　　　0x2表示二进制数据帧
	//　　　　0x3-7暂时无定义，为以后的非控制帧保留
	//　　　　0x8表示连接关闭
	//　　　　0x9表示ping
	//　　　　0xA表示pong
	//　　　　0xB-F暂时无定义，为以后的控制帧保留
	Mask   bool  //1位，用于标识PayloadData是否经过掩码处理，客户端发出的数据帧需要进行掩码处理，所以此位是1。数据需要解码。
	Length int64 //如果 x值在0-125，则是7位是payload的真实长度。
	//　　如果 x值是126，则后面2个字节形成的16位无符号整型数的值是payload的真实长度。
	//　　如果 x值是127，则后面8个字节形成的64位无符号整型数的值是payload的真实长度。
	MaskingKey []byte //32位的掩码
	data       []byte //消息体
}

func (f *Frame) Decode(rd *bufio.Reader) error {
	b, _ := rd.ReadByte()
	if b>>7 == 1 {
		f.Fin = true
	}
	f.OpCode = b & 0b0000_1111 //后4位为opCode
	b, _ = rd.ReadByte()
	if b>>7 == 1 {
		f.Mask = true
	}
	b = b & 0b0111_1111 //清掉第一位（也就是mask字段）
	if b <= 125 {
		f.Length = int64(b)
	}
	if b == 126 {
		bs := make([]byte, 2)
		rd.Read(bs)
		f.Length = parseByteToLen(bs)
	}
	if b == 127 {
		bs := make([]byte, 4)
		rd.Read(bs)
		f.Length = parseByteToLen(bs)
	}
	//读取掩码
	if f.Mask {
		f.MaskingKey = make([]byte, 4)
		rd.Read(f.MaskingKey) //4字节掩码
	}
	//根据len去read data
	f.data = make([]byte, f.Length)
	rd.Read(f.data)

	//解掩码
	if f.Mask {
		decodeData := make([]byte, f.Length)
		//这段逻辑也是rfc定义的标准解掩码格式
		for i := int64(0); i < f.Length; i++ {
			decodeData[i] = f.data[i] ^ f.MaskingKey[i%4]
		}
		f.data = decodeData
	}
	return nil
}

func (f *Frame) Encode() ([]byte, error) {
	bytes := []byte{}
	var b byte
	if f.Fin {
		b = byte(0b_1000_0000)
	}
	b = b | f.OpCode
	bytes = append(bytes, b)
	b = 0
	if f.Mask {
		b = byte(0b_1000_0000)
	}
	writeLenBytes := 0
	if f.Length <= 125 {
		b = b | byte(f.Length)
	} else if f.Length > 125 && f.Length <= 65535 {
		bytes = append(bytes, b|0b_1111_1110) //写入126
		writeLenBytes = 2                     //需要2bit
	} else {
		bytes = append(bytes, b|0b_1111_1111) //写入127
		writeLenBytes = 4                     //需要4bit
	}
	bytes = append(bytes, b)
	//长度写入
	bytes = append(bytes, parseLenToByte(f.Length, writeLenBytes)...)
	//marking写入
	if f.Mask {
		bytes = append(bytes, f.MaskingKey[:4]...) //32位的掩码
	}
	//data写入
	bytes = append(bytes, f.data...)
	return bytes, nil
}

// payload length的二进制表达采用网络序（big endian，低地址 存高位字节）。
func parseLenToByte(i int64, lenBytes int) []byte {
	if lenBytes == 0 {
		return nil
	}
	bytes := []byte{}
	for lenBytes != 0 {
		lenBytes--
		bytes = append(bytes, byte(i>>(lenBytes*8)))
	}
	return bytes
}

func parseByteToLen(bs []byte) int64 {
	length := int64(0)
	for _, b := range bs {
		length = length + int64(b)
		length = length << 8
	}
	return length
}
