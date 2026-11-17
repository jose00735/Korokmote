//go:build windows

package winplat

import (
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// Core Audio through hand-written COM vtable calls.

type guid struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

var (
	clsidMMDeviceEnumerator = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioEndpointVolume = guid{0x5CDF2C82, 0x841E, 0x4546, [8]byte{0x97, 0x22, 0x0C, 0xF7, 0x40, 0x78, 0x22, 0x9A}}
	pkeyDeviceFriendlyName  = propertyKey{guid{0xa45c254e, 0xdf1c, 0x4efd, [8]byte{0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0}}, 14}
)

type propertyKey struct {
	fmtid guid
	pid   uint32
}

type propVariant struct {
	vt  uint16
	_   [3]uint16
	val uintptr
	_   uintptr
}

var (
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

const (
	clsctxAll     = 0x17
	eRender       = 0
	eMultimedia   = 1
	deviceStateOn = 1
	vtLPWSTR      = 31
	coinitMulti   = 0x0
	ptrSize       = unsafe.Sizeof(uintptr(0))
)

// vcall invokes method idx of a COM object.
func vcall(obj unsafe.Pointer, idx uintptr, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*ptrSize))
	all := append([]uintptr{uintptr(obj)}, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		vcall(obj, 2)
	}
}

func hr(r uintptr, what string) error {
	if int32(r) < 0 {
		return fmt.Errorf("%s: HRESULT 0x%08X", what, uint32(r))
	}
	return nil
}

type audio struct {
	jobs chan func()
	enum unsafe.Pointer
	err  error
}

func newAudio() *audio {
	a := &audio{jobs: make(chan func())}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		procCoInitializeEx.Call(0, coinitMulti)
		var enum unsafe.Pointer
		r, _, _ := procCoCreateInstance.Call(
			uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
			uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum)))
		a.err = hr(r, "CoCreateInstance(MMDeviceEnumerator)")
		a.enum = enum
		close(ready)
		for job := range a.jobs {
			job()
		}
	}()
	<-ready
	return a
}

func (a *audio) do(f func() error) error {
	if a.err != nil {
		return a.err
	}
	done := make(chan error, 1)
	a.jobs <- func() { done <- f() }
	return <-done
}

func (a *audio) device(id string) (unsafe.Pointer, error) {
	var dev unsafe.Pointer
	if id == "" || id == "default" {
		r := vcall(a.enum, 4, eRender, eMultimedia, uintptr(unsafe.Pointer(&dev)))
		return dev, hr(r, "GetDefaultAudioEndpoint")
	}
	r := vcall(a.enum, 5, u16p(id), uintptr(unsafe.Pointer(&dev)))
	return dev, hr(r, "GetDevice")
}

func endpoint(dev unsafe.Pointer) (unsafe.Pointer, error) {
	var vol unsafe.Pointer
	r := vcall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioEndpointVolume)), clsctxAll, 0, uintptr(unsafe.Pointer(&vol)))
	return vol, hr(r, "Activate(IAudioEndpointVolume)")
}

func deviceID(dev unsafe.Pointer) string {
	var p uintptr
	if int32(vcall(dev, 5, uintptr(unsafe.Pointer(&p)))) < 0 {
		return ""
	}
	s := readU16(p)
	procCoTaskMemFree.Call(p)
	return s
}

func friendlyName(dev unsafe.Pointer) string {
	var store unsafe.Pointer
	if int32(vcall(dev, 4, 0, uintptr(unsafe.Pointer(&store)))) < 0 {
		return ""
	}
	defer release(store)
	var pv propVariant
	if int32(vcall(store, 5, uintptr(unsafe.Pointer(&pkeyDeviceFriendlyName)), uintptr(unsafe.Pointer(&pv)))) < 0 {
		return ""
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	if pv.vt != vtLPWSTR {
		return ""
	}
	return readU16(pv.val)
}

func readVolume(vol unsafe.Pointer) (float64, bool, error) {
	var level float32
	if err := hr(vcall(vol, 9, uintptr(unsafe.Pointer(&level))), "GetMasterVolumeLevelScalar"); err != nil {
		return 0, false, err
	}
	var muted int32
	if err := hr(vcall(vol, 15, uintptr(unsafe.Pointer(&muted))), "GetMute"); err != nil {
		return 0, false, err
	}
	return math.Round(float64(level)*1000) / 1000, muted != 0, nil
}

func (a *audio) withVolume(id string, f func(vol unsafe.Pointer) error) error {
	return a.do(func() error {
		dev, err := a.device(id)
		if err != nil {
			return err
		}
		defer release(dev)
		vol, err := endpoint(dev)
		if err != nil {
			return err
		}
		defer release(vol)
		return f(vol)
	})
}

func (a *audio) Devices() ([]proto.AudioDevice, error) {
	var out []proto.AudioDevice
	err := a.do(func() error {
		defID := ""
		if d, err := a.device("default"); err == nil {
			defID = deviceID(d)
			release(d)
		}
		var coll unsafe.Pointer
		if err := hr(vcall(a.enum, 3, eRender, deviceStateOn, uintptr(unsafe.Pointer(&coll))), "EnumAudioEndpoints"); err != nil {
			return err
		}
		defer release(coll)
		var n uint32
		if err := hr(vcall(coll, 3, uintptr(unsafe.Pointer(&n))), "GetCount"); err != nil {
			return err
		}
		for i := uint32(0); i < n; i++ {
			var dev unsafe.Pointer
			if int32(vcall(coll, 4, uintptr(i), uintptr(unsafe.Pointer(&dev)))) < 0 {
				continue
			}
			d := proto.AudioDevice{ID: deviceID(dev), Name: friendlyName(dev)}
			d.Default = d.ID == defID
			if vol, err := endpoint(dev); err == nil {
				d.Level, d.Muted, _ = readVolume(vol)
				release(vol)
			}
			release(dev)
			if d.Name == "" {
				d.Name = "Audio device"
			}
			out = append(out, d)
		}
		return nil
	})
	return out, err
}

func (a *audio) Volume(id string) (float64, bool, error) {
	var level float64
	var muted bool
	err := a.withVolume(id, func(vol unsafe.Pointer) error {
		var err error
		level, muted, err = readVolume(vol)
		return err
	})
	return level, muted, err
}

func (a *audio) SetVolume(id string, level float64) error {
	level = math.Max(0, math.Min(1, level))
	return a.withVolume(id, func(vol unsafe.Pointer) error {
		// The float travels in the integer slot; Go also copies it to XMM1 as the x64 ABI expects.
		return hr(vcall(vol, 7, uintptr(math.Float32bits(float32(level))), 0), "SetMasterVolumeLevelScalar")
	})
}

func (a *audio) SetMute(id string, muted bool) error {
	return a.withVolume(id, func(vol unsafe.Pointer) error {
		return hr(vcall(vol, 14, boolArg(muted), 0), "SetMute")
	})
}
