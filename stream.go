// set GOOGLE_APPLICATION_CREDENTIALS=PATH TO\googlecredentials.json
// $env:GOOGLE_APPLICATION_CREDENTIALS="PATH TO\googlecredentials.json"

package main

import (
	"C"
	
	"io"
	"log"
	"reflect"
	"unsafe"
	"bytes"
	"encoding/binary"
	
	auth "golang.org/x/oauth2"
	
	speech "cloud.google.com/go/speech/apiv1"
	"golang.org/x/net/context"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

var stream speechpb.Speech_StreamingRecognizeClient

//export InitializeStream
func InitializeStream() {
	
	ctx := context.Background()

	// [START speech_streaming_file_recognize]
	client, err := speech.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
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
					SampleRateHertz: 16000,
					LanguageCode:    "en-US",
				},
			},
		},
	}); err != nil {
		log.Fatal(err)
	}
}


//next comment needed by cgo to know which function to export
//export SpeechToText
func SpeechToText(recording *C.short, recording_len C.int) (*_Ctype_char) {
	
	// Create a slice of C.short values.
	// side-note: the slice references the input C.short values
	var length = int(recording_len) // convert recording_len from C.int to an int value (needed by functions in Golang
	var list []C.short
	sliceHeader := (*reflect.SliceHeader)((unsafe.Pointer(&list)))
	sliceHeader.Len = length
	sliceHeader.Cap = length
	sliceHeader.Data = uintptr(unsafe.Pointer(recording))
	
	// list to byte buffer (kopie)
	buftemp := new(bytes.Buffer)
	err := binary.Write(buftemp, binary.LittleEndian, list)
	
	if err != nil {
		log.Fatalf("binary.Write failed:", err)
	}	
	
	go func() {
		buf := make([]byte, 1024)

		for {
			//fill buffer
			n, err := buftemp.Read(buf)		
			
			if n > 0 {
				if err := stream.Send(&speechpb.StreamingRecognizeRequest{
					StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
						AudioContent: buf[:n],		
					},
				}); err != nil {
					log.Printf("Could not send audio: %v", err)
				}
			}
			if err == io.EOF {
				// Nothing else to pipe, close the stream.
				if err := stream.CloseSend(); err != nil {
					log.Fatalf("Could not close stream: %v", err)
				}
				return
			}
		}
	}()

	for {
	
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Cannot stream results: %v", err)
		}
		if err := resp.Error; err != nil {
			log.Fatalf("Could not recognize: %v", err)
		}
		for _, result := range resp.Results {
			for _, alternatives := range result.Alternatives {
				return C.CString(alternatives.Transcript)
			}
			
		}
	}

return C.CString("")
}

func main() { }