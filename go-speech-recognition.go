/*
	Author: Christopher Dreide (https://github.com/Drizzy3D)
	
	This C++ library written in Go provides functions needed to transcribe 
	speech to text using Google's "Cloud Speech-To-Text" API.
	It needs to be compiled with cgo:
	"go build -o go-speech-recognition.dll -buildmode=c-shared go-speech-recognition.go"
	
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
var cancel context.CancelFunc

var client* speech.Client
var stream speechpb.Speech_StreamingRecognizeClient

// Used to save error logs
var logStatus string;

// Used to safely close the stream
var sendMutex = &sync.Mutex{}
var receiveMutex = &sync.Mutex{}

var initialized = false


/*
	InitializeStream(cLanguage *_Ctype_char, cSampleRate C.int):
	one time initialization,
	sets the streaming session up (saved in global variables),
	sends the initial configuration message
	Parameter:
		cTranscriptLanguage *_Ctype_char
			(transcription language as a C string (use BCP-47 language tag))
		cSampleRate C.int
			(the sample rate of the audio recording as a C integer value, it's recommended
			use at least 16kHz)
		
	Return:
		1 if successful
		0 if failed (error log can be retrieved with "GetLog()")
*/

// Next comment is needed by cgo to know which function to export.
//export InitializeStream
func InitializeStream(cTranscriptLanguage *_Ctype_char, cSampleRate C.int, cTranscriptionModel *_Ctype_char, cMaxAlternatives C.int, cInterimResults C.int ) (C.int) {
	

	// converts the input C string to a go string (needed to send the initialization message)
	goTranscriptLanguage := C.GoString(cTranscriptLanguage)

	// converts the input C integer to a go integer (needed to send the initialization message)
	goSampleRate := int32(cSampleRate)

	// converts the input C string to a go string (needed to send the initialization message)
	goTranscriptionModel := C.GoString(cTranscriptionModel)

	// converts the input C integer to a go integer (needed to send the initialization message)
	goMaxAlternatives := int32(cMaxAlternatives)

	// "converts" the input C integer to a bool
	goInterimResults := int32(cInterimResults) == int32(1)


	// Set the context for the stream.
	ctx, cancel = context.WithCancel(context.Background())

	// Create a new Client.
	client, err := speech.NewClient(ctx)
	if err != nil {
		logStatus = err.Error()
		return C.int(0);
	}
	
	// Create a new Stream.
	stream, err = client.StreamingRecognize(ctx)
	if err != nil {
		logStatus = err.Error()
		return C.int(0);
	}

	// Send the initial configuration message.
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
				StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
					StreamingConfig: &speechpb.StreamingRecognitionConfig{
						Config: &speechpb.RecognitionConfig{
							Encoding:			speechpb.RecognitionConfig_LINEAR16,
							SampleRateHertz:	goSampleRate,				// Remember to use a recording with 16KHz sample rate.
							LanguageCode:		goTranscriptLanguage,		// Can be adjusted to language to be transcribed. (BCP-47)
							Model:				goTranscriptionModel,		// Can be either "video", "phone_call", "command_and_search", "default" (see https://cloud.google.com/speech-to-text/docs/basics)
							MaxAlternatives:	goMaxAlternatives,			// Maximum number of recognition hypotheses: Valid values are 0-30, 0 or 1 return only one							
							},
						InterimResults:	goInterimResults,	// boolean
						},
					},
				}); 
	err != nil {
		logStatus = err.Error()
		return C.int(0);
	}


	initialized = true
	return C.int(1);
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
		1 if successful
		0 if failed (error log can be retrieved with "GetLog()")
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
		return C.int(0)
	}	


// [SENDING]
	
	// For sending to google we declare a slice of bytes, that acts as a pipeline.
	// When it's too big, the streaming is too fast for google, so we cap it at 1024 byte.
	pipeline := make([]byte, 1024)

	for {
		// Each loop run: Fill pipeline with the next 1024 values of the byte buffer.
		// n is needed to keep track of the reading progress
		n, err := temporaryByteBuffer.Read(pipeline)		
		
		// Stop streaming when reaching the end of the input stream.
		if err == io.EOF {
			return C.int(1)
		}

		if n > 0 {
			
			// Ensure that the stream is initialized
			sendMutex.Lock()			
				// Check if the stream is initialized
				if initialized == false {

					sendMutex.Unlock()

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
			
			if err == context.Canceled {
				return C.int(1)
			}
			if err != nil {
				logStatus = ("Could not send audio:" + err.Error())
				return C.int(0)
			}
		}
	}
}


/*
	ReceiveTranscript (output **C.char) (C.int):	
	retrieves and saves the current final transcripts from Google
	
	After the call output contains the current final transcript 
	
	Parameters:
		output:
			The pointer which is used to store the current final transcript
				
	Return:
		1 if successful
		0 if failed (error log can be retrieved with "GetLog()")
*/

// Next comment is needed by cgo to know which function to export.
//export ReceiveTranscript
func ReceiveTranscript (output **C.char) (C.int) {

	// Ensure that the stream is initialized
	receiveMutex.Lock()
		// Check if the stream is initialized
		if initialized == false {
			receiveMutex.Unlock()
			logStatus = ("Stream is not initialized")
			return C.int(0)
		}
		// Check if there are results or errors yet.
		resp, err := stream.Recv()
	receiveMutex.Unlock()

	// Error handling.
	if err == context.Canceled {
		return C.int(1)
	}


	if err != nil {
		logStatus = ("Cannot stream results: " + err.Error())
		return C.int(0)
	}

	if err := resp.Error; err != nil {
		logStatus = ("Could not recognize: " + err.GetMessage())
		return C.int(0)
	}

	var helperString = "";

	// Check received message for results and store it in helperString.
	for _, result := range resp.Results {	
		// Needed to get only the transcription without additional informations i.e. "confidence".
		for _, alternative := range result.Alternatives { 
			// If the alternative string starts with a space - remove it
			if(len(alternative.Transcript) > 0 && alternative.Transcript[0] == " "[0]) {
				
				// Concatenate the alternatives, splitted by ';'
				helperString += alternative.Transcript[1:] + (string(';'))

			} else {

				// Concatenate the alternatives, splitted by ';'
				helperString += alternative.Transcript + (string(';'))
			}
		}		
	}
	
	// Fill output and remove semicolons in front/end

	// ";word;"" -> "word"
	if((helperString[0] == ";"[0]) && (helperString[len(helperString)-1] == ";"[0])){
		*output = C.CString(helperString[1:len(helperString)-1])
		return C.int(1)

	// "word;"" -> "word"
	}else if ((helperString[0] != ";"[0]) && (helperString[len(helperString)-1] == ";"[0])){
		*output = C.CString(helperString[:len(helperString)-1])
		return C.int(1)

	// ";word"" -> "word"
	}else if ((helperString[0] == ";"[0]) && (helperString[len(helperString)-1] != ";"[0])){
		*output = C.CString(helperString[1:])
		return C.int(1)
	}

	// "word"
	*output = C.CString(helperString)
	return C.int(1)
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
*/

// Next comment is needed by cgo to know which function to export.
//export CloseStream
func CloseStream () () {
	cancel()
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
