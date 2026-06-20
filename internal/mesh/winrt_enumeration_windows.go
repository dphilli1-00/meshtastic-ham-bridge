//go:build windows

package mesh

// Hand-written WinRT COM bindings for Windows.Devices.Enumeration pairing types.
// winrt-go doesn't include the Enumeration namespace, so these follow the same
// pattern as winrt-go's generated code (COM vtable structs + syscall.SyscallN).
//
// Interface IIDs from the Windows 10 SDK (Windows.Devices.Enumeration.idl).
// Parameterized-type GUIDs computed via the WinRT version-5 UUID algorithm.

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go/windows/foundation"
)

// ── Enums ────────────────────────────────────────────────────────────────────

// DevicePairingKinds is a flags enum; ConfirmOnly covers "Just Works" BLE.
type DevicePairingKinds uint32

const (
	DevicePairingKindsNone            DevicePairingKinds = 0
	DevicePairingKindsConfirmOnly     DevicePairingKinds = 1
	DevicePairingKindsDisplayPin      DevicePairingKinds = 2
	DevicePairingKindsProvidePin      DevicePairingKinds = 4
	DevicePairingKindsConfirmPinMatch DevicePairingKinds = 8
)

// DevicePairingResultStatus values we care about.
type DevicePairingResultStatus int32

const (
	DevicePairingResultStatusPaired                DevicePairingResultStatus = 0
	DevicePairingResultStatusAlreadyPaired         DevicePairingResultStatus = 3
	DevicePairingResultStatusRemoteDeviceHasAssociation DevicePairingResultStatus = 19
)

// ── IDeviceInformation (v1, partial) — only get_Id needed ────────────────
// IID {ABA0FB95-4398-489D-8E44-E6130927011F}
// The object from BluetoothLEDevice.DeviceInformation is a lightweight cached
// object; we use get_Id to obtain the device ID string, then call
// DeviceInformation.CreateFromIdAsync to get a full object with Pairing support.

const guidIDeviceInformationV1 = "aba0fb95-4398-489d-8e44-e6130927011f"

type iDeviceInformationV1 struct{ ole.IInspectable }

type iDeviceInformationV1Vtbl struct {
	ole.IInspectableVtbl
	GetId               uintptr // slot 6  → HSTRING
	GetName             uintptr // slot 7
	GetIsEnabled        uintptr // slot 8
	GetIsDefault        uintptr // slot 9
	GetEnclosureLocation uintptr // slot 10
	GetProperties       uintptr // slot 11
	Update              uintptr // slot 12
	GetThumbnailAsync   uintptr // slot 13
	GetGlyphThumbnailAsync uintptr // slot 14
}

func (v *iDeviceInformationV1) vtbl() *iDeviceInformationV1Vtbl {
	return (*iDeviceInformationV1Vtbl)(unsafe.Pointer(v.RawVTable))
}

// getId returns the device ID as a Go string and frees the underlying HSTRING.
func (v *iDeviceInformationV1) getId() (string, error) {
	var hstr uintptr
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetId,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&hstr)),
	)
	if hr != 0 {
		return "", ole.NewError(hr)
	}
	s, err := hstringToString(hstr)
	windowsDeleteString(hstr)
	return s, err
}

// ── IDeviceInformationStatics — CreateFromIdAsync ─────────────────────────
// IID {C17F100E-3A46-4A78-8013-769DC9B97390}
// Static factory on "Windows.Devices.Enumeration.DeviceInformation".
// CreateFromIdAsync is at vtable slot 6 and returns a FULL DeviceInformation
// object that supports IDeviceInformation2 (with the Pairing property).

const guidIDeviceInformationStatics = "c17f100e-3a46-4a78-8013-769dc9b97390"

type iDeviceInformationStatics struct{ ole.IInspectable }

type iDeviceInformationStaticsVtbl struct {
	ole.IInspectableVtbl
	CreateFromIdAsync                    uintptr // slot 6
	CreateFromIdAsyncAdditionalProperties uintptr // slot 7
	FindAllAsync                         uintptr // slot 8
	FindAllAsyncAqsFilter                uintptr // slot 9
	FindAllAsyncAqsFilterAndAdditionalProperties uintptr // slot 10
	CreateWatcher                        uintptr // slot 11
	CreateWatcherAqsFilter               uintptr // slot 12
	CreateWatcherAqsFilterAndAdditionalProperties uintptr // slot 13
}

func (v *iDeviceInformationStatics) vtbl() *iDeviceInformationStaticsVtbl {
	return (*iDeviceInformationStaticsVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iDeviceInformationStatics) createFromIdAsync(deviceId string) (*foundation.IAsyncOperation, error) {
	hstr, err := stringToHstring(deviceId)
	if err != nil {
		return nil, err
	}
	defer windowsDeleteString(hstr)

	var out *foundation.IAsyncOperation
	hr, _, _ := syscall.SyscallN(
		v.vtbl().CreateFromIdAsync,
		uintptr(unsafe.Pointer(v)),
		hstr,
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

// deviceInformationCreateFromIdAsync calls DeviceInformation.CreateFromIdAsync
// via the WinRT activation factory and returns a full DeviceInformation object.
func deviceInformationCreateFromIdAsync(deviceId string) (*foundation.IAsyncOperation, error) {
	factoryItf, err := ole.RoGetActivationFactory(
		"Windows.Devices.Enumeration.DeviceInformation",
		ole.NewGUID(guidIDeviceInformationStatics),
	)
	if err != nil {
		return nil, fmt.Errorf("RoGetActivationFactory DeviceInformation: %w", err)
	}
	statics := (*iDeviceInformationStatics)(unsafe.Pointer(factoryItf))
	defer statics.Release()
	return statics.createFromIdAsync(deviceId)
}

// ── HSTRING helpers ───────────────────────────────────────────────────────

var (
	modCombase           = syscall.NewLazyDLL("combase.dll")
	procWindowsCreateString    = modCombase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = modCombase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer = modCombase.NewProc("WindowsGetStringRawBuffer")
)

func stringToHstring(s string) (uintptr, error) {
	u16, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return 0, err
	}
	var hstr uintptr
	hr, _, _ := procWindowsCreateString.Call(
		uintptr(unsafe.Pointer(u16)),
		uintptr(len([]rune(s))),
		uintptr(unsafe.Pointer(&hstr)),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	return hstr, nil
}

func windowsDeleteString(hstr uintptr) {
	procWindowsDeleteString.Call(hstr)
}

func hstringToString(hstr uintptr) (string, error) {
	if hstr == 0 {
		return "", nil
	}
	var length uint32
	rawPtr, _, _ := procWindowsGetStringRawBuffer.Call(hstr, uintptr(unsafe.Pointer(&length)))
	if rawPtr == 0 || length == 0 {
		return "", nil
	}
	slice := (*[1 << 20]uint16)(unsafe.Pointer(rawPtr))[:length:length]
	return syscall.UTF16ToString(slice), nil
}

// ── iBluetoothLEDevice2 ───────────────────────────────────────────────────
// IID {26F062B3-7AEE-4D31-BABA-B1B9775F5916}
// winrt-go defines the struct but not GetDeviceInformation; we add it here.

const guidIBluetoothLEDevice2Enum = "26f062b3-7aee-4d31-baba-b1b9775f5916"

type iBLEDevice2ForPairing struct{ ole.IInspectable }

type iBLEDevice2ForPairingVtbl struct {
	ole.IInspectableVtbl
	GetDeviceInformation    uintptr // slot 6
	GetAppearance           uintptr // slot 7
	GetBluetoothAddressType uintptr // slot 8
}

func (v *iBLEDevice2ForPairing) vtbl() *iBLEDevice2ForPairingVtbl {
	return (*iBLEDevice2ForPairingVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iBLEDevice2ForPairing) getDeviceInformation() (*ole.IUnknown, error) {
	var out *ole.IUnknown
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetDeviceInformation,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

// ── IDeviceInformation2 ───────────────────────────────────────────────────
// IID {FDE4C6A1-F55A-4381-91FB-A41AD77AD7B5}

// Confirmed at runtime via GetIids() + vtable probe (slot 7 → DeviceInformationPairing).
const guidIDeviceInformation2 = "f156a638-7997-48d9-a10c-269d46533f48"

type iDeviceInformation2 struct{ ole.IInspectable }

type iDeviceInformation2Vtbl struct {
	ole.IInspectableVtbl
	GetKind    uintptr // slot 6
	GetPairing uintptr // slot 7
}

func (v *iDeviceInformation2) vtbl() *iDeviceInformation2Vtbl {
	return (*iDeviceInformation2Vtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iDeviceInformation2) getPairing() (*iDeviceInformationPairing, error) {
	var out *iDeviceInformationPairing
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetPairing,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

// ── IDeviceInformationPairing ─────────────────────────────────────────────
// IID {BA627726-6C7A-4A0A-9B8D-CB1089E5E2F9}

// Confirmed at runtime via GetIids() on the DeviceInformationPairing object.
const guidIDeviceInformationPairing = "2c4769f5-f684-40d5-8469-e8dbaab70485"

type iDeviceInformationPairing struct{ ole.IInspectable }

type iDeviceInformationPairingVtbl struct {
	ole.IInspectableVtbl
	GetIsPaired uintptr // slot 6
	GetCanPair  uintptr // slot 7
	PairAsync   uintptr // slot 8
	UnpairAsync uintptr // slot 9
}

func (v *iDeviceInformationPairing) vtbl() *iDeviceInformationPairingVtbl {
	return (*iDeviceInformationPairingVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iDeviceInformationPairing) getIsPaired() (bool, error) {
	var out uint8 // WinRT boolean = unsigned char
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetIsPaired,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return false, ole.NewError(hr)
	}
	return out != 0, nil
}

// pairAsync triggers the Windows system pairing dialog and returns an async op
// that resolves to a DevicePairingResult.
func (v *iDeviceInformationPairing) pairAsync() (*foundation.IAsyncOperation, error) {
	var out *foundation.IAsyncOperation
	hr, _, _ := syscall.SyscallN(
		v.vtbl().PairAsync,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

// ── IDeviceInformationPairing2 ────────────────────────────────────────────
// IID {D3BDA145-D44D-4A04-B289-5BB14614E23D}

// Confirmed at runtime via GetIids() on the DeviceInformationPairing object.
const guidIDeviceInformationPairing2 = "f68612fd-0aee-4328-85cc-1c742bb1790d"

type iDeviceInformationPairing2 struct{ ole.IInspectable }

type iDeviceInformationPairing2Vtbl struct {
	ole.IInspectableVtbl
	GetProtectionLevel                      uintptr // slot 6
	GetCustom                               uintptr // slot 7
	PairAsync                               uintptr // slot 8  (no args version on Pairing2)
	PairWithProtectionLevelAsync            uintptr // slot 9
	PairWithProtectionLevelAndSettingsAsync uintptr // slot 10
}

func (v *iDeviceInformationPairing2) vtbl() *iDeviceInformationPairing2Vtbl {
	return (*iDeviceInformationPairing2Vtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iDeviceInformationPairing2) getCustom() (*iDeviceInformationCustomPairing, error) {
	var out *iDeviceInformationCustomPairing
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetCustom,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

// ── IDeviceInformationCustomPairing ──────────────────────────────────────
// IID {630E7E9F-E4DA-4F41-8953-CE21A1373A2F}

// Confirmed at runtime via GetIids() on the DeviceInformationCustomPairing object.
const guidIDeviceInformationCustomPairing = "85138c02-4ee6-4914-8370-107a39144c0e"

// guidPairingRequestedHandler is the parameterized GUID for
// TypedEventHandler<DeviceInformationCustomPairing, DevicePairingRequestedEventArgs>,
// computed via the WinRT version-5 UUID algorithm from the type signature:
//
//	pinterface({9de1c534-6ae1-11e0-84e1-18a905bcc53f};
//	  rc(Windows.Devices.Enumeration.DeviceInformationCustomPairing;{85138c02-4ee6-4914-8370-107a39144c0e});
//	  rc(Windows.Devices.Enumeration.DevicePairingRequestedEventArgs;{f717fc56-de6b-487f-8376-0180aca69963}))
const guidPairingRequestedHandler = "fa65231f-4178-5de1-b2cc-03e22d7702b4"

type iDeviceInformationCustomPairing struct{ ole.IInspectable }

type iDeviceInformationCustomPairingVtbl struct {
	ole.IInspectableVtbl
	PairAsync                    uintptr // slot 6  PairAsync(DevicePairingKinds)
	PairWithProtectionLevelAsync uintptr // slot 7
	AddPairingRequested          uintptr // slot 8
	RemovePairingRequested       uintptr // slot 9
}

func (v *iDeviceInformationCustomPairing) vtbl() *iDeviceInformationCustomPairingVtbl {
	return (*iDeviceInformationCustomPairingVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iDeviceInformationCustomPairing) addPairingRequested(handler *foundation.TypedEventHandler) (foundation.EventRegistrationToken, error) {
	var token foundation.EventRegistrationToken
	hr, _, _ := syscall.SyscallN(
		v.vtbl().AddPairingRequested,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(handler)),
		uintptr(unsafe.Pointer(&token)),
	)
	if hr != 0 {
		return foundation.EventRegistrationToken{}, ole.NewError(hr)
	}
	return token, nil
}

func (v *iDeviceInformationCustomPairing) removePairingRequested(token foundation.EventRegistrationToken) {
	syscall.SyscallN(
		v.vtbl().RemovePairingRequested,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&token)),
	)
}

func (v *iDeviceInformationCustomPairing) pairAsync(kinds DevicePairingKinds) (*foundation.IAsyncOperation, error) {
	var out *foundation.IAsyncOperation
	hr, _, _ := syscall.SyscallN(
		v.vtbl().PairAsync,
		uintptr(unsafe.Pointer(v)),
		uintptr(kinds),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return out, nil
}

// ── IDevicePairingRequestedEventArgs ─────────────────────────────────────
// IID {F717FC56-DE6B-487F-8376-0180ACA69963}

const guidIDevicePairingRequestedEventArgs = "f717fc56-de6b-487f-8376-0180aca69963"

type iDevicePairingRequestedEventArgs struct{ ole.IInspectable }

type iDevicePairingRequestedEventArgsVtbl struct {
	ole.IInspectableVtbl
	GetDeviceInformation uintptr // slot 6
	GetPairingKind       uintptr // slot 7
	GetPin               uintptr // slot 8
	Accept               uintptr // slot 9
	AcceptWithPin        uintptr // slot 10  HSTRING pin
	GetDeferral          uintptr // slot 11
}

func (v *iDevicePairingRequestedEventArgs) vtbl() *iDevicePairingRequestedEventArgsVtbl {
	return (*iDevicePairingRequestedEventArgsVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iDevicePairingRequestedEventArgs) getPairingKind() (DevicePairingKinds, error) {
	var out DevicePairingKinds
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetPairingKind,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	return out, nil
}

func (v *iDevicePairingRequestedEventArgs) accept() error {
	hr, _, _ := syscall.SyscallN(
		v.vtbl().Accept,
		uintptr(unsafe.Pointer(v)),
	)
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}

func (v *iDevicePairingRequestedEventArgs) acceptWithPin(pin string) error {
	hpin, err := stringToHstring(pin)
	if err != nil {
		return err
	}
	defer windowsDeleteString(hpin)
	hr, _, _ := syscall.SyscallN(
		v.vtbl().AcceptWithPin,
		uintptr(unsafe.Pointer(v)),
		hpin,
	)
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}

// ── IDevicePairingResult ──────────────────────────────────────────────────
// IID {072B02BF-DD55-40E9-B7DD-D8C84AB8F4AC}

// Confirmed at runtime via GetIids() on the DevicePairingResult object.
const guidIDevicePairingResult = "072b02bf-dd95-4025-9b37-de51adba37b7"

type iDevicePairingResult struct{ ole.IInspectable }

type iDevicePairingResultVtbl struct {
	ole.IInspectableVtbl
	GetStatus              uintptr // slot 6
	GetProtectionLevelUsed uintptr // slot 7
}

func (v *iDevicePairingResult) vtbl() *iDevicePairingResultVtbl {
	return (*iDevicePairingResultVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iDevicePairingResult) getStatus() (DevicePairingResultStatus, error) {
	var out DevicePairingResultStatus
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetStatus,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	return out, nil
}

// ── IAsyncInfo ────────────────────────────────────────────────────────────
// IID {00000036-0000-0000-C000-000000000046}
// Used to poll completion status of any IAsyncOperation.

const guidIAsyncInfoEnum = "00000036-0000-0000-c000-000000000046"

type iAsyncInfoEnum struct{ ole.IInspectable }

type iAsyncInfoEnumVtbl struct {
	ole.IInspectableVtbl
	GetId        uintptr // slot 6
	GetStatus    uintptr // slot 7
	GetErrorCode uintptr // slot 8
	Cancel       uintptr // slot 9
	Close        uintptr // slot 10
}

func (v *iAsyncInfoEnum) vtbl() *iAsyncInfoEnumVtbl {
	return (*iAsyncInfoEnumVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *iAsyncInfoEnum) getStatus() (int32, error) {
	var out int32
	hr, _, _ := syscall.SyscallN(
		v.vtbl().GetStatus,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&out)),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	return out, nil
}

// ── Diagnostic helpers ────────────────────────────────────────────────────

// winrtGetIIDs returns all interface IIDs an IInspectable object supports,
// via IInspectable.GetIids (vtable slot 3). Useful for debugging QI failures.
func winrtGetIIDs(unk *ole.IUnknown) ([]string, error) {
	type vtbl3 struct {
		QueryInterface uintptr
		AddRef         uintptr
		Release        uintptr
		GetIids        uintptr
	}
	vt := (*vtbl3)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(unk))))
	var count uint32
	var iids uintptr // GUID* (CoTaskMemAlloc'd array)
	hr, _, _ := syscall.SyscallN(vt.GetIids,
		uintptr(unsafe.Pointer(unk)),
		uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&iids)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	if iids == 0 || count == 0 {
		return nil, nil
	}
	result := make([]string, count)
	guidSize := unsafe.Sizeof(ole.GUID{})
	for i := uint32(0); i < count; i++ {
		g := (*ole.GUID)(unsafe.Pointer(iids + uintptr(i)*guidSize))
		result[i] = fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
			g.Data1, g.Data2, g.Data3,
			g.Data4[0], g.Data4[1],
			g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
	}
	syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree").Call(iids)
	return result, nil
}

// winrtRuntimeClassName returns the WinRT runtime class name of any IInspectable.
// Calls IInspectable.GetRuntimeClassName (vtable slot 4, zero-indexed).
func winrtRuntimeClassName(unk *ole.IUnknown) (string, error) {
	type vtbl4 struct {
		QueryInterface      uintptr
		AddRef              uintptr
		Release             uintptr
		GetIids             uintptr
		GetRuntimeClassName uintptr
	}
	vt := (*vtbl4)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(unk))))
	var hstr uintptr
	hr, _, _ := syscall.SyscallN(vt.GetRuntimeClassName,
		uintptr(unsafe.Pointer(unk)),
		uintptr(unsafe.Pointer(&hstr)),
	)
	if hr != 0 {
		return "", ole.NewError(hr)
	}
	s, err := hstringToString(hstr)
	windowsDeleteString(hstr)
	return s, err
}

// probeDevInfo2GetPairing safely tries calling slot 7 (get_Pairing) on a
// candidate IDeviceInformation2 interface. Uses recover() to survive crashes
// from calling the wrong vtable slot on an unrelated interface.
// Returns (pairing, runtimeClassName, error).
func probeDevInfo2GetPairing(candidate *iDeviceInformation2) (result *iDeviceInformationPairing, name string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			result = nil
		}
	}()
	p, pErr := candidate.getPairing()
	if pErr != nil {
		return nil, "", pErr
	}
	if p == nil {
		return nil, "", fmt.Errorf("nil result")
	}
	pUnk := (*ole.IUnknown)(unsafe.Pointer(p))
	name, nameErr := winrtRuntimeClassName(pUnk)
	if nameErr != nil {
		pUnk.Release()
		return nil, "", fmt.Errorf("GetRuntimeClassName: %w", nameErr)
	}
	return p, name, nil
}

// ── Async helpers ─────────────────────────────────────────────────────────

// awaitAsyncOp polls IAsyncInfo.Status until the operation completes (status=1),
// then calls GetResults() and returns the raw result pointer.
// Returns error on timeout, cancellation, or async error.
func awaitAsyncOp(op *foundation.IAsyncOperation, timeout time.Duration) (unsafe.Pointer, error) {
	infoItf, err := op.QueryInterface(ole.NewGUID(guidIAsyncInfoEnum))
	if err != nil {
		return nil, fmt.Errorf("QI IAsyncInfo: %w", err)
	}
	info := (*iAsyncInfoEnum)(unsafe.Pointer(infoItf))
	defer info.Release()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := info.getStatus()
		if err != nil {
			return nil, fmt.Errorf("IAsyncInfo.Status: %w", err)
		}
		switch status {
		case 1: // AsyncStatus.Completed
			return op.GetResults()
		case 2: // AsyncStatus.Canceled
			return nil, fmt.Errorf("async operation canceled")
		case 3: // AsyncStatus.Error
			return nil, fmt.Errorf("async operation error")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out after %v", timeout)
}

// awaitPairingResult awaits an IAsyncOperation<DevicePairingResult> and returns
// the DevicePairingResultStatus.
func awaitPairingResult(op *foundation.IAsyncOperation) (DevicePairingResultStatus, error) {
	resultPtr, err := awaitAsyncOp(op, 30*time.Second)
	if err != nil {
		return 0, err
	}
	if resultPtr == nil {
		return 0, fmt.Errorf("PairAsync returned nil result")
	}
	// GetResults() returns the raw DevicePairingResult COM pointer.
	// QI for IDevicePairingResult to call GetStatus.
	resultUnk := (*ole.IUnknown)(resultPtr)
	if iids, iidErr := winrtGetIIDs(resultUnk); iidErr == nil {
		fmt.Printf("BLE: DevicePairingResult IIDs (%d):\n", len(iids))
		for _, id := range iids {
			fmt.Printf("  %s\n", id)
		}
	}
	resultItf, err := resultUnk.QueryInterface(ole.NewGUID(guidIDevicePairingResult))
	if err != nil {
		return 0, fmt.Errorf("QI IDevicePairingResult: %w", err)
	}
	result := (*iDevicePairingResult)(unsafe.Pointer(resultItf))
	defer result.Release()
	return result.getStatus()
}
