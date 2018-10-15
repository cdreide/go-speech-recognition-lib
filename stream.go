/*
	Author: Christopher Dreide (https://github.com/Drizzy3D)
	
	This C++ library written in Go provides functions needed to transcribe 
	speech to text using Google's "Cloud Speech-To-Text" API.
	It needs to be compiled with cgo:
	"go build -o libgostream.dll -buildmode=c-shared stream.go"
	
	See the README.md for instructions on how to use this library.
*/

package main // Needs to remain main package for cgo compiling.

import (

	// Needed to feature cgo compatibility
	"C"
	
	// Standard packages:
	"io"
	"log"
	"reflect"
	"unsafe"
	"bytes"
	"encoding/binary"
	"context"

	// External (Google) packages (download with "go get -u cloud.google.com/go/speech/apiv1"):
	speech "cloud.google.com/go/speech/apiv1"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

// Global variables needed to maintain the session (and to feature an one time initialization).
var ctx context.Context
var client* speech.Client
var stream speechpb.Speech_StreamingRecognizeClient

/*
	InitializeStream(cLanguage *_Ctype_char):
	one time initialization,
	sets the streaming session up (saved in global variables),
	sends the initial configuration message

	Parameter:

		cTranscriptLanguage *_Ctype_char
			(transcription language as a C string (use BCP-47 language tag))

*/

// Next comment is needed by cgo to know which function to export.
//export InitializeStream
func InitializeStream(cTranscriptLanguage *_Ctype_char) {
	
	// converts the input C string to a go string (needed to send the initialization message)
	goTranscriptLanguage := C.GoString(cTranscriptLanguage)

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
							SampleRateHertz: 16000,						// Remember to use a recording with 16KHz sample rate.
							LanguageCode:    goTranscriptLanguage,		// Can be adjusted to language to be transcribed.
							},
						},
					},
				}); 
	err != nil {
		log.Fatal(err)
	}
}

	
/*
	SendAudio(recording, recordingLength C.int):
	prepares the inputted audio data to be sent to google,
	handles the sending process
	
	Parameters:

		recording:
			has to be a Pointer to short values committed by C++ function call
			(16KHz Audio Stream)
		
		recordingLength:
			just the length of the recording (needed as we can't use C++ vectors in golang)	
*/
	
// Next comment is needed by cgo to know which function to export.
//export SendAudio
func SendAudio(recording *C.short, recordingLength C.int){

	// Create a slice of C.short values.
	var length = int(recordingLength) 	// Convert recordingLength from C.int to an int value (needed to define the sliceHeader in the following).
	var list []C.short			// Define a new slice of C.shorts.
	
	// Pass the reference to the input C.short values to the slice's data.
	sliceHeader := (*reflect.SliceHeader)((unsafe.Pointer(&list)))
	sliceHeader.Len = length
	sliceHeader.Cap = length
	sliceHeader.Data = uintptr(unsafe.Pointer(recording))
	
	// As we need to send byte values instead of C.Shorts, the list gets copied in a temporary bytes.Buffer.
	// (maybe changed in future for reduction of copy operations)
	temporaryByteBuffer := new(bytes.Buffer)
	err := binary.Write(temporaryByteBuffer, binary.LittleEndian, list)
	
	if err != nil {
		log.Fatalf("binary.Write failed:", err)
	}	


// [SENDING]
	
	// For sending to google we declare a slice of bytes, that acts as a pipeline.
	// When it's too big, the streaming is too fast for google, so we cap it at 1024 byte.
	pipeline := make([]byte, 1024)

	for {
		// Each loop run: Fill pipeline with the next 1024 values of the byte buffer.
		// n is needed to keep track of the reading progress
		n, err := temporaryByteBuffer.Read(pipeline)		
		
		if n > 0 {
			// Send the pipeline upto the n-th byte (except the last loop run n==1024) as a message to google
			if err := stream.Send(&speechpb.StreamingRecognizeRequest{
					StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
						AudioContent: pipeline[:n],		
						},
				});
			err != nil {
				log.Printf("Could not send audio: %v", err)
			}
		}
		// Stop streaming when reaching the end of the input stream.
		if err == io.EOF {
			return
		}	
	}

}



/*
	ReceiveTranscript ()  (*_Ctype_char):	
	retrieves and returns the current final transcripts from Google

	returns: 
		*_Ctype_char
		(can be received as a string in C++)
	
*/

// Next comment is needed by cgo to know which function to export.
//export ReceiveTranscript
func ReceiveTranscript ()  (*_Ctype_char) {

	// Check if there are results or errors yet.
	resp, err := stream.Recv()

	// Error handling.
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
			transcript := result.Transcript
			return C.CString(transcript)
		}		
	}

	// Nothing has been transcribed - therefore we return an empty CString.
	return C.CString("")
}

// For the sake of completeness (because cgo forces us to declare a main package), we need a main function.
func main() {}
