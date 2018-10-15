# go-speech-recognition-lib

This library, written in GoLang, provides functions needed to transcribe speech to text using Google's "Cloud Speech-To-Text" API.
It is designed to be used in a C++ Program and to provide an easy to use C++ Speech-To-Text solution.
	 

## Getting Started

The following instructions will help on how to compile and use the go library in a C++ program.

### Prerequisites
	
First you'll need the Go compiler (for now you have to use the 64 Bit version):
```
https://golang.org/dl/
```

To use the API you need to get the Client library ("go get" uses git so make sure you run the command in the git bash or the "git" environment variable has been set):
```
go get -u cloud.google.com/go/speech/apiv1
```
	
To use the "Cloud Speech-To-Text" API you need an API-Key (see [Google How-To](https://cloud.google.com/speech-to-text/docs/quickstart-client-libraries#before-you-begin)).
You need to set the GOOGLE_APPLICATION_CREDENTIALS environment variable to point to your google credentials file (containing your API key):	
cmd:
```
set GOOGLE_APPLICATION_CREDENTIALS=PATH TO\googlecredentials.json
```
PowerShell:
```
$env:GOOGLE_APPLICATION_CREDENTIALS="PATH TO\googlecredentials.json"
```
(support of retrieving the API key directly from the .json might be added in future)

As we are using the cgo compiler (part of the Go compiler installed in the first step) to compile a C++ library  from Go code,
you'll need the [MinGW-w64](https://mingw-w64.org/doku.php/download) compiler (used by cgo).

	
Last but not least: of course you will need a C++ Program to use the library:
You'll need an 16KHz audiostream represented by a std::vector containing short values and get String represantation of the transcript.
	
	
### Prepare the C++

We will load the library at runtime, so you'll have perform a few more steps than just including a header file.
To achieve this we'll use Windows' HANDLES.
	
First you'll have to add:
```
#include <windows.h>
```
	
Then we declare the type of the function handles we will use (global):
```
typedef char*(*INITIALIZE_STREAM)();
typedef char*(*SEND_AUDIO)(const short*, int);
typedef char*(*RECEIVE_TRANSCRIPT)();
```
	
Next we want to load the plugin (our .dll). Add this to your function:
```
plugin_handle = LoadLibrary("libgostream.dll");
```
	
With a quick check we ensure that the plugin is loaded properly (else you have to leave the function to avoid runtime errors):
```
if (!plugin_handle)
{
	std::cout << "Could not load plugin." << std::endl;
	return;
}
```
	
When the plugin is loaded we have to load the function handles (it's recommend to declare these variables global to prevent scope problems while using threaded calls):

```
INITIALIZE_STREAM InitializeStream;
SEND_AUDIO SendAudio;
RECEIVE_TRANSCRIPT ReceiveTranscript;
```					

```
InitializeStream = reinterpret_cast<INITIALIZE_STREAM>(GetProcAddress(plugin_handle, "InitializeStream"));
SendAudio = reinterpret_cast<SEND_AUDIO>(GetProcAddress(plugin_handle, "SendAudio"));
ReceiveTranscript = reinterpret_cast<RECEIVE_TRANSCRIPT>(GetProcAddress(plugin_handle, "ReceiveTranscript"));
```					
	
Ensure that the plugins have been loaded properly (else you have to leave the function to avoid runtime errors):
```	
if (!InitializeStream)
{
	std::cout << "Could not load function InitializeStream." << std::endl;
	return
}
if (!SendAudio)
{
	std::cout << "Could not load function SendAudio." << std::endl;
	return;
}
if (!ReceiveTranscript)
{
	std::cout << "Could not load function ReceiveTranscript." << std::endl;
	return;
}
```
	
And now you are able to call the functions provided by the library.
First initialize the Stream (provide a [BCP-47](https://www.rfc-editor.org/rfc/bcp/bcp47.txt) language tag to set the language to be transcribed):
```
InitializeStream("en-US");
```
	
Then call the send and receive functions (it's recommended to send and receive parallel in seperate threads):
```
SpeechToText(pointerToRecording->data(), pointerToRecording->size());
```

```
std::string received = ReceiveTranscript();
```
Note: size has to be an Integer representing the sample count of the recording.
	
### Installing the library

Now we are ready to compile the source code to a .dll file.
	
Compile with (run in the same directory as stream.go): 
```
go build -o libgostream.dll -buildmode=c-shared stream.go
```

In the end you'll have to copy the "libgostream.dll" in the same directory as your compiled C++ program and run your executable.	
		

## Authors

* **Christopher Dreide** - [Drizzy3D](https://github.com/Drizzy3D)

