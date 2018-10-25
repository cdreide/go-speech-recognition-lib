/*
	Author: Christopher Dreide (https://github.com/Drizzy3D)
	
	This C++ library written in Go provides functions needed to transcribe 
	speech to text using Google's "Cloud Speech-To-Text" API.
	It needs to be compiled with cgo:
	"go build -o go-speech-recognition-lib.dll -buildmode=c-shared go-speech-recognition-lib.go"
	
	See the README.md for instructions on how to use this library.
*/

package main // Needs to remain main package for cgo compiling.

import (
	
	"C" // Needed to feature cgo compatibility
	
	// Standard packages:
	"io"
	"reflect"
	"unsafe"
	"bytes"
	"encoding/binary"
	"context"
	"sync"
 
	// External (Google) packages (download with "go get -u cloud.google.com/go/speech/apiv1"):
	speech "cloud.google.com/go/speech/apiv1"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

// Global variables needed to maintain the session (and to feature an one time initialization).
var ctx context.Context
var client* speech.Client
var stream speechpb.Speech_StreamingRecognizeClient

// Used to save error logs
var logStatus string;

// Used to safely close the stream
var sendMutex = &sync.Mutex{}
var receiveMutex = &sync.Mutex{}

var initialized = false


/*
	InitializeStream(cLanguage *_Ctype_char):
	one time initialization,
	sets the streaming session up (saved in global variables),
	sends the initial configuration message
	Parameter:
		cTranscriptLanguage *_Ctype_char
			(transcription language as a C string (use BCP-47 language tag))
	
	Return:
		0 if successful
		1 if failed (error log can be retrieved with "GetLog()")
*/

// Next comment is needed by cgo to know which function to export.
//export InitializeStream
func InitializeStream(cTranscriptLanguage *_Ctype_char, cSampleRate C.int) (C.int) {
	
	// converts the input C integer to a go integger (needed to send the initialization message)
	goSampleRate := int32(cSampleRate)

	// converts the input C string to a go string (needed to send the initialization message)
	goTranscriptLanguage := C.GoString(cTranscriptLanguage)

	// Set the context for the stream.
	ctx = context.Background()

	// Create a new Client.
	client, err := speech.NewClient(ctx)
	if err != nil {
		logStatus = err.Error()
		return C.int(1);
	}
	
	// Create a new Stream.
	stream, err = client.StreamingRecognize(ctx)
	if err != nil {
		logStatus = err.Error()
		return C.int(1);
	}

	// Send the initial configuration message.
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
				StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
					StreamingConfig: &speechpb.StreamingRecognitionConfig{
						Config: &speechpb.RecognitionConfig{
							Encoding:        speechpb.RecognitionConfig_LINEAR16,
							SampleRateHertz: goSampleRate,						// Remember to use a recording with 16KHz sample rate.
							LanguageCode:    goTranscriptLanguage,		// Can be adjusted to language to be transcribed.
							},
						},
					},
				}); 
	err != nil {
		logStatus = err.Error()
		return C.int(1);
	}


	initialized = true
	return C.int(0);
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

	Return:
		0 if successful
		1 if failed (error log can be retrieved with "GetLog()")
*/
	
// Next comment is needed by cgo to know which function to export.
//export SendAudio
func SendAudio(recording *C.short, recordingLength C.int) (C.int){

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
		logStatus = ("binary.Write failed:" + err.Error())
		return C.int(1)
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
			
			// Ensure that the stream is initialized
			sendMutex.Lock()			
			// Check if the stream is initialized
			if initialized == false {
				logStatus = ("Stream is not initialized")
				return C.int(1)
			}	
			// Send the pipeline upto the n-th byte (except the last loop run n==1024) as a message to google
			err := stream.Send(&speechpb.StreamingRecognizeRequest{
					StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
						AudioContent: pipeline[:n],		
						},
					});
			sendMutex.Unlock()
			if err != nil {
				logStatus = ("Could not send audio:" + err.Error())
				return C.int(1)
			}
		}
		// Stop streaming when reaching the end of the input stream.
		if err == io.EOF {
			logStatus = err.Error()
			return C.int(0)
		}	
	}

}


/*
	ReceiveTranscript ()  (*_Ctype_char):	
	retrieves and returns the current final transcripts from Google
	Return: 
		*_Ctype_char
		(can be received as a string in C++)
*/

// Next comment is needed by cgo to know which function to export.
//export ReceiveTranscript
func ReceiveTranscript ()  (*_Ctype_char) {

	// Ensure that the stream is initialized
	receiveMutex.Lock()
		// Check if the stream is initialized
		if initialized == false {
			logStatus = ("Stream is not initialized")
			return C.CString("")
		}
		// Check if there are results or errors yet.
		resp, err := stream.Recv()
	receiveMutex.Unlock()

	// Error handling.
	if err != nil {
		logStatus = ("Cannot stream results: " + err.Error())
	}

	if err := resp.Error; err != nil {
		logStatus = ("Could not recognize: " + err.GetMessage())
	}

	// Check received message for results and return it as a C.CString.
	for _, transcribed := range resp.Results {	
		// Needed to get only the transcription without additional informations i.e. "confidence".
		for _, result := range transcribed.Alternatives { 
			return C.CString(result.Transcript)
		}		
	}

	// Nothing has been transcribed - therefore we return an empty CString.
	return C.CString("")
}


/*
	GetLog () (*_Ctype_char)
	returns the last logged event as a String

	Return:
		logStatus as a CString (usable by C)
*/

// Next comment is needed by cgo to know which function to export.
//export GetLog
func GetLog () (*_Ctype_char) {
	return C.CString(logStatus);
}


/*
	CloseStream () (C.int):
	closes the streaming session

	Return:
		0 if successful
*/

// Next comment is needed by cgo to know which function to export.
//export CloseStream
func CloseStream () () {
	
	// Ensure that no sending or receiving is done while closing the stream.
	sendMutex.Lock()
	receiveMutex.Lock()
		stream = nil
		client = nil
		ctx = nil
		initialized = false
	receiveMutex.Unlock()	
	sendMutex.Unlock()
}


/*
	IsInitialized () (C.int)
	returns the status of initialization

	Return:
		1 if the stream is initialized
		0 if the stream is not initialized
*/

// Next comment is needed by cgo to know which function to export.
//export IsInitialized
func IsInitialized () (C.int) {
	if initialized == true {
		return C.int(1)
	} else {
		return C.int(0)
	}
}

// For the sake of completeness (because cgo forces us to declare a main package), we need a main function.
func main() {}
