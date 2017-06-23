gonut - A Go implementation of FFmpeg's nut container format
============================================================

NUT is a generic container format supported by FFmpeg. One useful feature of NUT is it can be used as an output format for FFmpeg's pipe protocol.

With gonut you to can exec an ffmpeg process and stream the resulting frames into your go application via stdout. You might use this to convert an h264 stream into raw RGBA and then in go into a series of image.Image objects.
