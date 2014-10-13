/* This file contains the utility functions that are required by both client and server */
/* I have manipulated raw bytes for constructing packets */
/* This can also be done by creating structs for each packet type and encoding/decoding structs directly */

package main
import (
	"encoding/binary"
	"bytes"
	"strings"
)

/* Initial Packet - 2 byte opcode, n byte filename, 0, method (octet), 0 */
/* Assumption - Length of initial packet can be upto only 516 bytes */
/* Meaning, filename can be only upto 507 bytes long */

func constructInitialPacket(opcode uint16, fileName string) ([]byte){

    fileNameLen := len(fileName)
	packetSize := 9 + fileNameLen
	initialPacket := make([]byte, packetSize)
	var index int = 0
	opcodeBuf := make([]byte, 2)
    binary.LittleEndian.PutUint16(opcodeBuf, uint16(opcode))
	initialPacket[index] = opcodeBuf[0]
	initialPacket[index+1] = opcodeBuf[1]
	index += 2
	fileNameBuf := []byte(fileName)
	for i := 0; i < fileNameLen; i++ {
        initialPacket[index] = fileNameBuf[i]
		index += 1
    } 
	initialPacket[index] = 0
	index += 1
	modeBuf := []byte("octet")
    for j := 0; j < 5; j++ {
        initialPacket[index] = modeBuf[j]
		index += 1
    } 	
	initialPacket[index] = 0
	return initialPacket
}

/* Data Packet - 2 bytes opcode, 2 bytes blocknum, 512 bytes data payload */
func constructDataPacket(blockNum uint16, data []byte, dataLen int) ([]byte) {

    var index int = 0
	packetSize := 4 + dataLen
	dataPacket := make([]byte, packetSize)
	var opcode uint16 = 3
	opcodeBuf := make([]byte, 2)
    binary.LittleEndian.PutUint16(opcodeBuf, uint16(opcode))
	dataPacket[index] = opcodeBuf[0]
	dataPacket[index+1] = opcodeBuf[1]
	index += 2
	blockBuf := make([]byte, 2)
    binary.LittleEndian.PutUint16(blockBuf, uint16(blockNum))
	dataPacket[index] = blockBuf[0]
	dataPacket[index+1] = blockBuf[1]	
	index += 2
	for i := 0; i < dataLen; i++ {
        dataPacket[index] = data[i]
		index += 1
    } 
	return dataPacket
}

/* Ack Packet - 2 byte opcode, 2 byte blockBuf */
func constructAckPacket(opcode uint16, blockNum uint16) ([]byte) {

    ackPacket := make([]byte, 4)
    opcodeBuf := make([]byte, 2)
    binary.LittleEndian.PutUint16(opcodeBuf, uint16(opcode))
	ackPacket[0] = opcodeBuf[0]
	ackPacket[1] = opcodeBuf[1]
    blockBuf := make([]byte, 2)
    binary.LittleEndian.PutUint16(blockBuf, uint16(blockNum))
	ackPacket[2] = blockBuf[0]
	ackPacket[3] = blockBuf[1]
    return ackPacket	
}

/* Error Packet - 2 byte opcode, 2 byte errorNum, n byte error string, 0 */
/* Assumption: Error String should be within 511 bytes */

func constructErrorPacket(opcode uint16) ([]byte) {
    errorPacket := make([]byte, 516)
	opcodeBuf := make([]byte, 2)
    binary.LittleEndian.PutUint16(opcodeBuf, uint16(opcode))
	var index int = 0
	errorPacket[index] = opcodeBuf[0]
	errorPacket[index+1] = opcodeBuf[1]
	index += 2
	var errorNum uint16 = 1
	errorNumBuf := make([]byte, 2)
    binary.LittleEndian.PutUint16(errorNumBuf, uint16(errorNum))
	errorPacket[index] = errorNumBuf[0]
	errorPacket[index+1] = errorNumBuf[1]
	index += 2
	errorStr := []byte("error")
	for i := 0; i < 5; i++ {
        errorPacket[index] = errorStr[i]
		index += 1
	}	
	errorPacket[index] = 0
	return errorPacket
}

func getOpcode(ingressBuf []byte) (uint16) {

    opcodeBuf := make([]byte, 2)      
	opcodeBuf[0] = ingressBuf[0]
	opcodeBuf[1] = ingressBuf[1]
	var opcode uint16
	err := binary.Read(bytes.NewReader(opcodeBuf), binary.LittleEndian, &opcode)
	check(err)
	return opcode
}
func getBlockNum(ingressBuf []byte) (uint16) {

    blockNumBuf := make([]byte, 2)      
	blockNumBuf[0] = ingressBuf[2]
	blockNumBuf[1] = ingressBuf[3]
	var blockNum uint16
	err := binary.Read(bytes.NewReader(blockNumBuf), binary.LittleEndian, &blockNum)
	check(err)
	return blockNum
}
func convertDataIngressBufType(ingressBuf [516]byte) ([]byte){

    ingressByte := make([]byte, 516)
	for i := 0; i < 516; i++ {
        ingressByte[i] = ingressBuf[i]
    }	
	return ingressByte
}
func convertAckIngressBufType(ingressBuf [4]byte) ([]byte){

    ingressByte := make([]byte, 4)
	for i := 0; i < 4; i++ {
        ingressByte[i] = ingressBuf[i]
    }	
	return ingressByte
}
func getFileName(ingressByte []byte) (string) {

    fileNameBuf := string(ingressByte)
    var fileName string
	lastIndex := strings.Index(fileNameBuf, "octet") - 2
    for i := 2; i <= lastIndex; i++ {
        fileName = fileName+string(fileNameBuf[i])
	}    
	return fileName
}
func getIngressData(ingressByte []byte, ingressBufSize int) ([]byte) {

    dataBufSize := ingressBufSize - 4
	dataBuf := make([]byte, dataBufSize)      
    for j := 0; j < dataBufSize; j++ {
        dataBuf[j] = ingressByte[j+4]
    }	
	return dataBuf
}
func check(e error) {

    if e != nil {
        panic(e)
    }
}
