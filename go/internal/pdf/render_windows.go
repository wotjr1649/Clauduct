//go:build windows && amd64

// Package pdf renders pages with the installed Windows PDF engine. The helper
// uses memory streams only: no document scripts, filenames, shell or network.
package pdf

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const MaxPages = 100
const MaxBytes = 24 << 20

// RenderArg is the whole argument list of the gateway's renderer subprocess, which is
// clauduct.exe run again. Reserved with a prefix no native option carries (#112).
const RenderArg = "--clauduct-render-pdf"

var ErrRender = errors.New("PDF_RENDER_FAILED")

type com struct{ table *[32]uintptr }

// Keep Go out-parameters alive and on the heap for the complete native call.
// Conversion is at each call site, as required by the compiler's syscall ABI.
//
//go:uintptrescapes
func (p *com) call(index int, args ...uintptr) error {
	if p == nil {
		return ErrRender
	}
	result, _, _ := syscall.SyscallN(p.table[index], append([]uintptr{uintptr(unsafe.Pointer(p))}, args...)...)
	runtime.KeepAlive(p)
	if int32(result) < 0 {
		return fmt.Errorf("PDF_RENDER_HRESULT_%08X", uint32(result))
	}
	return nil
}
func (p *com) release() {
	if p != nil {
		syscall.SyscallN(p.table[2], uintptr(unsafe.Pointer(p)))
	}
}

var iidStream = syscall.GUID{Data1: 0x0c, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var iidPDF = syscall.GUID{Data1: 0x433a0b5f, Data2: 0xc007, Data3: 0x4788, Data4: [8]byte{0x90, 0xf2, 0x08, 0x14, 0x3d, 0x92, 0x25, 0x99}}
var iidAsync = syscall.GUID{Data1: 0x36, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

// Only fixed system DLLs are loaded, with LOAD_LIBRARY_SEARCH_SYSTEM32.
func systemDLL(name string) (*syscall.DLL, error) {
	loader := syscall.NewLazyDLL("kernel32.dll").NewProc("LoadLibraryExW")
	wide, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, _, err := loader.Call(uintptr(unsafe.Pointer(wide)), 0, 0x800)
	if handle == 0 {
		return nil, err
	}
	return &syscall.DLL{Name: name, Handle: syscall.Handle(handle)}, nil
}

//go:uintptrescapes
func invoke(dll *syscall.DLL, name string, args ...uintptr) error {
	proc, err := dll.FindProc(name)
	if err != nil {
		return ErrRender
	}
	result, _, _ := proc.Call(args...)
	if int32(result) < 0 {
		return fmt.Errorf("PDF_RENDER_HRESULT_%08X", uint32(result))
	}
	return nil
}
func wait(ctx context.Context, operation *com) error {
	var info *com
	if err := operation.call(0, uintptr(unsafe.Pointer(&iidAsync)), uintptr(unsafe.Pointer(&info))); err != nil {
		return err
	}
	defer info.release()
	timer := time.NewTicker(time.Millisecond)
	defer timer.Stop()
	for {
		var status uint32
		if err := info.call(7, uintptr(unsafe.Pointer(&status))); err != nil {
			return err
		}
		if status == 1 {
			return nil
		}
		if status != 0 {
			return ErrRender
		}
		select {
		case <-ctx.Done():
			_ = info.call(9)
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// ABI declarations are Microsoft's Windows.Data.Pdf and Storage.Streams
// interfaces. No PDF parsing or guessed image-token formula lives here.
func Render(ctx context.Context, data []byte) (pages []string, err error) {
	return RenderRange(ctx, data, 1, 0, 192)
}

func RenderRange(ctx context.Context, data []byte, first, last, dpi int) (pages []string, err error) {
	if len(data) < 5 || len(data) > MaxBytes || string(data[:5]) != "%PDF-" {
		return nil, ErrRender
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rt, err := systemDLL("combase.dll")
	if err != nil {
		return nil, err
	}
	defer rt.Release()
	core, err := systemDLL("shcore.dll")
	if err != nil {
		return nil, err
	}
	defer core.Release()
	if err = invoke(rt, "RoInitialize", 1); err != nil {
		return nil, err
	}
	defer invoke(rt, "RoUninitialize")
	name, _ := syscall.UTF16FromString("Windows.Data.Pdf.PdfDocument")
	var text uintptr
	if err = invoke(rt, "WindowsCreateString", uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)-1), uintptr(unsafe.Pointer(&text))); err != nil {
		return nil, err
	}
	defer invoke(rt, "WindowsDeleteString", text)
	var factory *com
	if err = invoke(rt, "RoGetActivationFactory", text, uintptr(unsafe.Pointer(&iidPDF)), uintptr(unsafe.Pointer(&factory))); err != nil {
		return nil, err
	}
	defer factory.release()
	stream, source, err := memoryStreams(rt, core)
	if err != nil {
		return nil, err
	}
	defer source.release()
	defer stream.release()
	var written uint32
	if err = source.call(4, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written))); err != nil {
		return nil, err
	}
	runtime.KeepAlive(data)
	if int(written) != len(data) {
		return nil, ErrRender
	}
	if err = source.call(5, 0, 0, 0); err != nil {
		return nil, err
	}
	var operation *com
	if err = factory.call(8, uintptr(unsafe.Pointer(stream)), uintptr(unsafe.Pointer(&operation))); err != nil {
		return nil, err
	}
	defer operation.release()
	if err = wait(ctx, operation); err != nil {
		return nil, err
	}
	var document *com
	if err = operation.call(8, uintptr(unsafe.Pointer(&document))); err != nil {
		return nil, err
	}
	defer document.release()
	var count uint32
	if err = document.call(7, uintptr(unsafe.Pointer(&count))); err != nil {
		return nil, err
	}
	if last == 0 {
		last = int(count)
	}
	if first < 1 || first > last || last > int(count) {
		return nil, fmt.Errorf("Wrong page range given: last page (%d)", count)
	}
	if count == 0 || last-first+1 > MaxPages || dpi < 36 || dpi > 300 {
		return nil, errors.New("PDF_PAGE_LIMIT")
	}
	total := 0
	for index := uint32(first - 1); index < uint32(last); index++ {
		page, renderErr := renderPage(ctx, rt, core, document, index, dpi)
		if renderErr != nil {
			return nil, renderErr
		}
		total += len(page)
		if total > MaxBytes {
			return nil, errors.New("PDF_RENDER_SIZE_LIMIT")
		}
		pages = append(pages, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(page))
	}
	return pages, nil
}

func renderPage(ctx context.Context, rt, core *syscall.DLL, document *com, index uint32, dpi int) ([]byte, error) {
	var page *com
	if err := document.call(6, uintptr(index), uintptr(unsafe.Pointer(&page))); err != nil {
		return nil, err
	}
	defer page.release()
	var size struct{ Width, Height float32 }
	if err := page.call(10, uintptr(unsafe.Pointer(&size))); err != nil {
		return nil, err
	}
	if !(size.Width > 0 && size.Height > 0 && size.Width < 1e6 && size.Height < 1e6) {
		return nil, ErrRender
	}
	name, _ := syscall.UTF16FromString("Windows.Data.Pdf.PdfPageRenderOptions")
	var text uintptr
	if err := invoke(rt, "WindowsCreateString", uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)-1), uintptr(unsafe.Pointer(&text))); err != nil {
		return nil, err
	}
	defer invoke(rt, "WindowsDeleteString", text)
	var options *com
	if err := invoke(rt, "RoActivateInstance", text, uintptr(unsafe.Pointer(&options))); err != nil {
		return nil, err
	}
	defer options.release()
	width, height := float64(size.Width)*float64(dpi)/96, float64(size.Height)*float64(dpi)/96
	if width > 2048 || height > 2048 {
		scale := 2048 / max(width, height)
		width *= scale
		height *= scale
	}
	if err := options.call(9, uintptr(max(1, width))); err != nil {
		return nil, err
	}
	if err := options.call(11, uintptr(max(1, height))); err != nil {
		return nil, err
	}
	stream, sink, err := memoryStreams(rt, core)
	if err != nil {
		return nil, err
	}
	defer sink.release()
	defer stream.release()
	var operation *com
	if err := page.call(7, uintptr(unsafe.Pointer(stream)), uintptr(unsafe.Pointer(options)), uintptr(unsafe.Pointer(&operation))); err != nil {
		return nil, err
	}
	defer operation.release()
	if err := wait(ctx, operation); err != nil {
		return nil, err
	}
	if err := operation.call(8); err != nil {
		return nil, err
	}
	var sizeBytes uint64
	if err := stream.call(6, uintptr(unsafe.Pointer(&sizeBytes))); err != nil {
		return nil, err
	}
	if sizeBytes < 8 || sizeBytes > MaxBytes {
		return nil, ErrRender
	}
	if err := sink.call(5, 0, 0, 0); err != nil {
		return nil, err
	}
	out := make([]byte, sizeBytes)
	var read uint32
	if err := sink.call(3, uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)), uintptr(unsafe.Pointer(&read))); err != nil {
		return nil, err
	}
	if uint64(read) != sizeBytes || string(out[:8]) != "\x89PNG\r\n\x1a\n" {
		return nil, ErrRender
	}
	return out, nil
}

// Out-parameters keep native addresses as typed pointers instead of holding a
// pointer in an integer across calls. Both interfaces refer to the same memory.
func memoryStreams(rt, core *syscall.DLL) (stream, classic *com, err error) {
	name, _ := syscall.UTF16FromString("Windows.Storage.Streams.InMemoryRandomAccessStream")
	var text uintptr
	if err = invoke(rt, "WindowsCreateString", uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)-1), uintptr(unsafe.Pointer(&text))); err != nil {
		return
	}
	defer invoke(rt, "WindowsDeleteString", text)
	if err = invoke(rt, "RoActivateInstance", text, uintptr(unsafe.Pointer(&stream))); err != nil {
		return
	}
	if err = invoke(core, "CreateStreamOverRandomAccessStream", uintptr(unsafe.Pointer(stream)), uintptr(unsafe.Pointer(&iidStream)), uintptr(unsafe.Pointer(&classic))); err != nil {
		stream.release()
		stream = nil
	}
	return
}
