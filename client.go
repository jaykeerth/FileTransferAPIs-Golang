package main
import (
    "os"
    "fmt"
    "net"
	"strings"
	"bytes"
	"time"
)
func main() {

    if len(os.Args) != 2 {
		fmt.Println("Usage Example -> 'go run client.go tftpUtilities.go RequestType:InputFileName:OutputFileName' ")
		os.Exit(1)
	}
	userInput := os.Args[1]
	parameters := strings.Split(userInput, ":") 
	requestType := parameters[0]
	inputFileName := parameters[1]
	outputFileName := parameters[2]
	
	/* Control Channel */	
	
    service := "127.0.0.1:1201"
	udpAddr, err := net.ResolveUDPAddr("udp", service)
    check(err)
	controlChannel, err := net.DialUDP("udp", nil, udpAddr)
    check(err)
	if requestType == "read" {
	    handleReadRequest(controlChannel, inputFileName, outputFileName)
	} else {
	    handleWriteRequest(controlChannel, inputFileName, outputFileName) 
	}
}

/* Handler for read requests to the server */ 

func handleReadRequest(controlChannel *net.UDPConn, inputFileName string, outputFileName string) {  

	fmt.Println("Sending Read request.")
    initialPacket := constructInitialPacket(1, inputFileName)
	clientAddr:= controlChannel.LocalAddr()
	clientAddrStr := clientAddr.String()
	clientPort := strings.Split(clientAddrStr, ":")
	fmt.Println("Client Port is : ", clientPort[1])
    _, errInitialPk := controlChannel.Write(initialPacket)
    check(errInitialPk)
    controlChannel.Close()         
	
	/* Data Channel */
	
	var ingressBuf [516]byte
    var ingressBufSize int
    var serverAddr *net.UDPAddr
    var clientDataBuf bytes.Buffer
	var prevBlockNum uint16 = 0
    var blockNum uint16 = 0
	var lastPacket bool = false
	
	newService := "127.0.0.1:"+clientPort[1]
	newudpAddr, err := net.ResolveUDPAddr("udp", newService)
	check(err)
	dataChannel, err := net.ListenUDP("udp", newudpAddr)
	check(err) 
    
    /* Setting the read timeout limit for all data packets from the server to 8 seconds */	
	dataChannel.SetReadDeadline(time.Now().Add(time.Second * 8))
	
	for {
	        /* For the last data packet, wait for additional 8 seconds after it has been sent to the server. */
            /* This handles the case when the last packet has been sent by client but not received by server. */
            /* If the server resends the Ack for previous data packet, client sends the last data packet again */
            /* File is created only after the entire content is read from the server */
			
		    if lastPacket == true {
			   _, _, err := dataChannel.ReadFromUDP(ingressBuf[0:])
			   if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				   fileWrite, err := os.Create(outputFileName)
	               check(err)
		           _, errOutput := fileWrite.WriteString(clientDataBuf.String())
	               check(errOutput)
		           fileWrite.Close()
				   fmt.Println("File has been fully read from the server into the current directory.")
				   break
			   }
			} else {
	               ingressBufferSize, remoteAddr, err := dataChannel.ReadFromUDP(ingressBuf[0:])
				   if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
			           fmt.Println("Server timed out. Closing connection. Try again.")
				       break
			       }
				   ingressBufSize = ingressBufferSize
				   serverAddr = remoteAddr
			}
			ingressByte := convertDataIngressBufType(ingressBuf)
			opcode := getOpcode(ingressByte)
			/* Received Error Packet from the server */
			if opcode == 5 {
			   fmt.Println("Data transfer did not succeed. Closing connection. Try again.")
			   break
			}
			blockNum = getBlockNum(ingressByte)
			fmt.Println("Received Data Block ", blockNum)
			
			/* Storing only unique data blocks in the buffer */ 
			/* If Data is received and stored but if Ack did not reach the server, */
			/* data will be resent from server. In this case, no need to store it in the buffer again. */
			
			if prevBlockNum < blockNum {                    
                ingressDataBuf := getIngressData(ingressByte, ingressBufSize)
                clientDataBuf.Grow(len(ingressDataBuf)) 			
	            _, err := clientDataBuf.Write(ingressDataBuf)
	            check(err)
			}
			
			/* If Ack from client did not reach the server, server will timeout and send prev data packet again. */
            /* So send the Ack for the prev data block again to ensure that server will move onto the next data packet */
			
			prevBlockNum = blockNum
			ackBuf := constructAckPacket(4, prevBlockNum)
			_, errWr1 := dataChannel.WriteToUDP(ackBuf, serverAddr)
            check(errWr1)
			fmt.Println("Sent Ack for block: ", prevBlockNum)
			if ingressBufSize < 516 {
			    lastPacket = true
			}	
		}
		dataChannel.Close()
        os.Exit(0)
}

/* Handler for write requests to the server */

func handleWriteRequest(controlChannel *net.UDPConn, inputFileName string, outputFileName string) {  

    fmt.Println("Sending write request.")
    initialPacket := constructInitialPacket(2, outputFileName)
	clientAddr:= controlChannel.LocalAddr()
	clientAddrStr := clientAddr.String()
	clientPort := strings.Split(clientAddrStr, ":")
	fmt.Println("Client Port is : ", clientPort[1])
    _, errWrite := controlChannel.Write(initialPacket)
    check(errWrite)
    controlChannel.Close()
	
	/* Data Channel */
	
	var ingressBuf [4]byte
	prevDataPacket := make([]byte, 512)
	var serverAddr *net.UDPAddr
	var expectedBlockNum uint16 = 0
	var lastPacket bool = false
	var firstAck bool = true
	var retryCount int = 1
	var closeConn bool = false
	
	newService := "127.0.0.1:"+clientPort[1]
	newudpAddr, err := net.ResolveUDPAddr("udp", newService)
	check(err)
	dataChannel, err := net.ListenUDP("udp", newudpAddr)
	check(err) 
	fileRead, err := os.Open(inputFileName) 
    check(err)
	
	/* Read timeout for Ack from the server is set to 4 seconds */
	dataChannel.SetReadDeadline(time.Now().Add(time.Second * 4))
    
	/* The first Ack from the server is for block 0. It is to start the data transfer from the client. */
	/* If first Ack did not reach the client within the timeout period, datachannel client connection is closed */
	/* For other Acks, the previous data packet is retransmitted upto 4 times after the timeouts before closing the connection. */
	
	for {
	    for { 
	        _, remoteAddr, err := dataChannel.ReadFromUDP(ingressBuf[0:])
		    if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
			    if firstAck == false {
				    if retryCount == 4 {
					   closeConn = true
					   break
					}
					_, errWr := dataChannel.WriteToUDP(prevDataPacket, remoteAddr)
                    check(errWr)
					retryCount += 1
				} else {
				    fmt.Println("Server timed out. Closing connection. Try again.") 
					closeConn = true
					break
				}
			} else {
                serverAddr = remoteAddr			   
			    break
			}
		}
		if closeConn == true {
		    break
		}
		ingressByte := convertAckIngressBufType(ingressBuf)
		opcode := getOpcode(ingressByte)
		if opcode != 4 {
		   fmt.Println("Data transfer did not succeed. Closing connection. Try again.")
		   break
		}
		blockNum := getBlockNum(ingressByte)
		fmt.Println("Received Ack for block: ", blockNum)
		firstAck = false
		
		/* When the Ack for last packet is received, client successfully closes the connection */
		if blockNum == expectedBlockNum {
		   if lastPacket == true {
		      fmt.Println("File has been successfully written to the server.")
		      break
		   }
		   inputBuf := make([]byte, 512)
           inputBufSize, err := fileRead.Read(inputBuf)
		   /* If there is a file read failure send error packet to server */
		   if err != nil {
		       errorPacket := constructErrorPacket(5)
			   _, errToServer := dataChannel.WriteToUDP(errorPacket, serverAddr)
               check(errToServer)
			   break
		   }
		   expectedBlockNum = expectedBlockNum+1
		   dataPacket := constructDataPacket(expectedBlockNum, inputBuf, inputBufSize)
		   _, errWrite := dataChannel.WriteToUDP(dataPacket, serverAddr)
           check(errWrite)
		   if len(dataPacket) < 516 {
		      lastPacket = true
		   }
		   fmt.Println("Sent data block ", expectedBlockNum)
		   prevDataPacket = make([]byte, len(dataPacket))
		   prevDataPacket = dataPacket
		} else {
		   fmt.Println("Data transfer did not succeed. Closing connection. Try again.")
		   break
		}
	}
	fileRead.Close()
	dataChannel.Close()
    os.Exit(0)
}

