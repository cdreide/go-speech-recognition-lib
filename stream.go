/*
	Author: Christopher Dreide (https://github.com/Drizzy3D)
	
	This C++ library written in Go provides functions needed to transcribe 
	speech to text using Google's "Cloud Speech-To-Text" API.
	
	See the README.md for instructions on how to use the library.
*/

package main // Needs to remain main package for cgo compiling.

import (
	"C"
	
	// Standard packages:
	"io"
	"log"
	"reflect"
	"unsafe"
	"bytes"
	"encoding/binary"
	"context"
//	"time"
	"fmt"
	
	// External (Google) packages:
	speech "cloud.google.com/go/speech/apiv1"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

// Global variables needed to maintain the session (currently only partially working).
var ctx context.Context
var client* speech.Client
var stream speechpb.Speech_StreamingRecognizeClient

/*
	InitializeStream():
	sets the streaming session up (saved in global varaibles),
	sends the initial configuration message
*/

// Next comment i needed by cgo to know which function to export.
//export InitializeStream
func InitializeStream() {
	
	// Set the context for the stream.
	ctx = context.Background()

	// Create a new Client.
	client, err := speech.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	
	// Create a new Stream.
	stream, err = client.StreamingRecognize(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Send the initial configuration message.
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
				StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
					StreamingConfig: &speechpb.StreamingRecognitionConfig{
						Config: &speechpb.RecognitionConfig{
							Encoding:        speechpb.RecognitionConfig_LINEAR16,
							SampleRateHertz: 16000,		// Remember to use a recording with 16KHz sample rate.
							LanguageCode:    "en-US",	// Can be adjusted to language to be transcribed.
							},
						},
					},
				}); 
	err != nil {
		log.Fatal(err)
	}
}


	
/*
	SpeechToText(recording, recordingLength C.int) (*_Ctype_char)
	
	recording:
	has to be a Pointer to short values committed by C++ function call
	(16KHz Audio Stream)
	
	recordingLength:
	just the length of the recording (needed as we can't use C++ vectors in golang)	
	returns: *_Ctype_char
	(can be received as a string in C++)
	
*/
	
// Next comment is needed by cgo to know which function to export.
//export SendAudio
func SendAudio(recording *C.short, recordingLength C.int){

	// Create a slice of C.short values.
	var length = int(recordingLength) // Convert recordingLength from C.int to an int value (needed to define the sliceHeader in the following).
	var list []C.short				// Define a new slice of C.shorts.
	
	// Pass the reference to the input C.short values to the slice's data.
	sliceHeader := (*reflect.SliceHeader)((unsafe.Pointer(&list)))
	sliceHeader.Len = length
	sliceHeader.Cap = length
	sliceHeader.Data = uintptr(unsafe.Pointer(recording))
	
	// As we need to send byte values instead of C.Shorts, the list gets copied in a temporary bytes.Buffer.
	// (maybe changed in future for reduction of copy operations)
	temporaryByteBuffer := new(bytes.Buffer)
	err := binary.Write(temporaryByteBuffer, binary.LittleEndian, list)
	
	fmt.Printf("%v \n",length)
	
	if err != nil {
		log.Fatalf("binary.Write failed:", err)
	}	
	
// [SENDING]
	
	// Start of a goroutine, so we can send the recording while parallel waiting for the results.

		// For sending to google we declare a slice of bytes, that acts as a pipeline.
		// When it's too big, the streaming is too fast for google, so we cap it at 1024 byte.
		pipeline := make([]byte, 1024)

		for {
			// Fill pipeline with the first 1024 values of the byte buffer.
			// n is needed to keep track of the reading progress
			n, err := temporaryByteBuffer.Read(pipeline)		
			fmt.Printf("%v \n", n)
			// Close stream when reaching the end of the input stream.
			if err == io.EOF {
			//	if err := stream.CloseSend(); err != nil {
			//		log.Fatalf("Could not close stream: %v", err)
			//	}
				return
			}
			fmt.Printf("sends \n")
				// Send the pipeline upto the n-th byte (except the last loop run applies n==1024) as a message to google
				if err := stream.Send(&speechpb.StreamingRecognizeRequest{
							StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
								AudioContent: pipeline[:n],		
								},
						});
				err != nil {
					log.Printf("Could not send audio: %v", err)
				}
			
		}
}

// [RECEIVING]

// Next comment is needed by cgo to know which function to export.
//export ReceiveTranscript
func ReceiveTranscript ()  (*_Ctype_char) {

	// Safety check that stream is already initialized
//	for(stream == nil){
	//	time.Sleep(200 * time.Nanosecond)
	//}
	fmt.Printf("in ReceiveTranscript")
	// Check if there are results or errors yet (happens parallel to the sending part).

		

		resp, err := stream.Recv()
		
		fmt.Printf("after recv")
		// Error handling.
		if err == io.EOF {
			return C.CString("")
		}
		fmt.Printf("afterEOF")
		if err != nil {
			log.Fatalf("Cannot stream results: %v", err)
		}
		if err := resp.Error; err != nil {
			log.Fatalf("Could not recognize: %v", err)
		}
		// Check received message for results and return it as a C.CString.
		for _, transcribed := range resp.Results {	
			// Needed to get only the transcription without additional informations i.e. "confidence".
			for _, result := range transcribed.Alternatives { 
				// TODO: Handle encoding failures that might happen with some transcribed languages.
				return C.CString(result.Transcript)
			}		
		}
	

	// Nothing has been transcribed - therefore we return an empty CString.
	return C.CString("")
}

// For the sake of completeness (because cgo forces us to declare a main package), we need a main function.
func main() {}
