package main
import (
	"fmt"
	"net"
	"os"
	"strconv"
	"bytes"
	"time"
)
func main() {

	service := "127.0.0.1:1201"
	udpAddr, err := net.ResolveUDPAddr("udp", service)
	if err != nil {
		fmt.Println(os.Stderr, "Error occurred: ", err.Error())
		os.Exit(0)
	} 

	/* Server Control Channel */
	
	controlChannel, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println(os.Stderr, "Error occurred: ", err.Error())
		os.Exit(0)
	} 
	for {
		handleClient(controlChannel)
	}
}
func handleClient(controlChannel *net.UDPConn) {

    var buf [516]byte
    _, clientAddr, err := controlChannel.ReadFromUDP(buf[0:])            
	if err != nil {
		return
	}
    /* When a request comes from a client, a separate thread is created using goroutine */
    /* Allows multiple clients to concurrently send requests to the server in the control channel */	
	
	go handleClientUtil(controlChannel, clientAddr, buf)
}

/* Goroutine for each client request */ 

func handleClientUtil(controlChannel *net.UDPConn, clientAddr *net.UDPAddr, buf [516]byte) {

    bufByte := convertDataIngressBufType(buf)
    opcode := getOpcode(bufByte)
	
	/* Server discards any packets with opcode other than RRQ (1) and WRQ (2) in the control channel */ 

	if opcode != 1 && opcode != 2 {
	    return
	}
	clientPort := strconv.Itoa(clientAddr.Port)
    newService := "127.0.0.1:"+clientPort
    newudpAddr, err := net.ResolveUDPAddr("udp", newService)
    if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", err.Error())
		return
	} 
	
	/* Creating Data Channel with a random server port and same client port */
    
	dataChannel, err := net.DialUDP("udp", nil, newudpAddr)
    if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", err.Error())
		return
	} 
    fmt.Println("New data channel opened at :", newService)	
	fileName := getFileName(bufByte)
	if opcode == 1 {
	    handleClientReadRequest(dataChannel, fileName)
	} else {
	    handleClientWriteRequest(dataChannel, fileName)
	}
}

/* Handler for processing Read requests from the client */

func handleClientReadRequest(dataChannel *net.UDPConn, fileName string) {
	
	fmt.Println("Handling client read request.")
	var ingressBuf [4]byte
	var expectedBlockNum uint16 = 0
	var lastPacket bool = false
	var retryCount int = 1
	var closeConn bool = false
	prevDataPacket := make([]byte, 512)
	
    dataChannel.SetReadDeadline(time.Now().Add(time.Second * 10))
	fileRead, err := os.Open(fileName) 
    if err != nil {
	    fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", err.Error())
	    dataChannel.Close()
        return
	} 
	for {
	    inputBuf := make([]byte, 512)
        inputBufSize, err := fileRead.Read(inputBuf)
		/* Send error packet if file read fails */
		if err != nil {
		    errorPacket := constructErrorPacket(5)
			_, errToClient := dataChannel.Write(errorPacket)
			if errToClient != nil {
			    fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", errToClient.Error())
			    break
			}
		    fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", err.Error())
		    break
	    } 
		
		/* expectedBlockNum is the block number of the data packet that is being sent from the server */
		/* It is the block number that is expected to be Acknowledged by the client */
		
		expectedBlockNum = expectedBlockNum+1
		dataPacket := constructDataPacket(expectedBlockNum, inputBuf, inputBufSize)
		_, errWrite := dataChannel.Write(dataPacket)
        if errWrite != nil {
		    fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", errWrite.Error())
		    break
	    } 
		fmt.Println("Sent data block num: ", expectedBlockNum)
		
		/* If data packet is less than 516 bytes, it is the last packet */
		
		if len(dataPacket) < 516 {
		   lastPacket = true
		}
		prevDataPacket = make([]byte, len(dataPacket))
		prevDataPacket = dataPacket
		
		/* Previous data packet is retransmitted upto 4 times after read timeout for Ack for client */ 
	    for { 
	        _, _, err := dataChannel.ReadFromUDP(ingressBuf[0:])
		    if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				if retryCount == 4 {
				    fmt.Println("Client timed out. Closing client connection. Try again.")
				    closeConn = true
					break
				}
				_, errWr := dataChannel.Write(prevDataPacket)
                if errWr != nil {
		            fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", errWr.Error())
		            closeConn = true
					break
	            } 
				fmt.Println("Sent block num: ", expectedBlockNum)
				retryCount += 1
			} else {
			    ingressByte := convertAckIngressBufType(ingressBuf)
		        opcode := getOpcode(ingressByte)
		        
				/* Allow only Ack packets from client on data channel for read request */ 
				if opcode != 4 {
		            closeConn = true
					break
		        }
		        blockNum := getBlockNum(ingressByte)
		        fmt.Println("Received Ack for block: ", blockNum)
				
				/* In TFTP, Data block is sent only after Ack is received for prev packet */
				/* So, Ack for any block other than the expected block is not allowed */
				
                if blockNum != expectedBlockNum {
				    closeConn = true
					break
				} else {         /* If correct Ack is received */
				    if lastPacket == true {     /* If that Ack is for the last packet, close the client connection successfully */ 
					    fmt.Println("Client has fully read the file from the server.")
						closeConn = true
						break
					} else {     /* If it is not last packet, go back to the top and send another data packet */
					    break
					}				    
				}
			}
		}
		if closeConn == true {
		    break
		}
	}	
	fileRead.Close()
	dataChannel.Close()
    return
}

/* Handler for processing write requests from the client */

func handleClientWriteRequest(dataChannel *net.UDPConn, fileName string) {

    fmt.Println("Handling client write request.")
	var ingressBuf [516]byte
    var ingressBufSize int
    var clientDataBuf bytes.Buffer
	var prevBlockNum uint16 = 0
    var blockNum uint16 = 0
	var lastPacket bool = false
	dataChannel.SetReadDeadline(time.Now().Add(time.Second * 18))
    
	/* Send Ack for block 0 to start data transfer from the client */
	
	ackBuf := constructAckPacket(4, 0)
	_, errAck := dataChannel.Write(ackBuf)
    if errAck != nil {
	    fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", errAck.Error())
		dataChannel.Close()
		return
	}
	fmt.Println("Ack sent for block 0")
	
	/* Ack is not retransmitted. If Ack gets lost, the client will retransmit the previous data packet again */
	
	for {
	        /* If last data block is received and Ack is sent by the server but not received by the client, */
			/* Client will retransmit the last data block again. So, wait for a few seconds before closing connection. */
			
		    if lastPacket == true {
			   dataChannel.SetReadDeadline(time.Now().Add(time.Second * 5))
			   _, _, err := dataChannel.ReadFromUDP(ingressBuf[0:])
			   if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				   fileWrite, err := os.Create(fileName)
	               if err != nil {
		               fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", err.Error())
					   break
	               } 
		           _, errOutput := fileWrite.WriteString(clientDataBuf.String())
	               if errOutput != nil {
		               fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", errOutput.Error())
					   fileWrite.Close()
					   break
	               } 
		           fileWrite.Close()
				   fmt.Println("File has been successfully written by the server into the current directory.")
				   break
			   }
			} else {   
	               ingressBufferSize, _, err := dataChannel.ReadFromUDP(ingressBuf[0:])
				   ingressBufSize = ingressBufferSize
			       if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
			           fmt.Println("Client timed out. Closing client connection. Try again.")
				       break
			       }
			}
			ingressByte := convertDataIngressBufType(ingressBuf)
			opcode := getOpcode(ingressByte)
			
			/* Received error packet from client */ 				
			if opcode == 5 {
			   fmt.Println("Data transfer did not succeed. Closing client connection. Try again.")
			   break
			}
			blockNum = getBlockNum(ingressByte)
			fmt.Println("Received Data Block ", blockNum)
			
			/* Storing only unique data blocks in the buffer */ 
			/* If Data is received and stored but if Ack did not reach the client, */
			/* data will be resent from client. In this case, no need to store it in the buffer again. */
			
			if prevBlockNum < blockNum {                   
                ingressDataBuf := getIngressData(ingressByte, ingressBufSize)
                clientDataBuf.Grow(len(ingressDataBuf)) 			
	            _, errBufWr := clientDataBuf.Write(ingressDataBuf)
	            if errBufWr != nil {
		               fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", errBufWr.Error())
					   break
	            } 
			}
			
			/* If Ack from server did not reach the client, client wil timeout and send prev data packet again. */
            /* So send the Ack for the prev data block again to ensure that client will move onto the next data packet */
			
			prevBlockNum = blockNum
			ackBuf := constructAckPacket(4, prevBlockNum)
			_, errWr := dataChannel.Write(ackBuf)
            if errWr != nil {
		        fmt.Fprintf(os.Stderr, "Error occurred during client transaction: ", errWr.Error())
		    	break
	        } 
			fmt.Println("Ack sent for block ",prevBlockNum)
			if ingressBufSize < 516 {
			    lastPacket = true
			}
	    }
		dataChannel.Close()
		return
}
