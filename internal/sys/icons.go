package sys

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// FileIcon is the icon of the program at path, px pixels square,
// premultiplied as a ui.Bitmap takes it; nil if it has none.
func FileIcon(path string, px int) *image.RGBA {
	p := winapi.UTF16(path)
	if p == nil {
		return nil
	}
	icon := winapi.SHDefExtractIcon(p, 0, uint32(px))
	if icon == 0 {
		return nil
	}
	defer winapi.DestroyIcon(icon)
	return iconPixels(icon, px)
}

// GenericIcon is the shell's icon for programs, as Task Manager shows for
// one without its own.
func GenericIcon(px int) *image.RGBA {
	var info winapi.StockIconInfo
	if !winapi.SHGetStockIconLocation(winapi.SIIDApplication, &info) {
		return nil
	}
	icon := winapi.SHDefExtractIcon(&info.Path[0], info.IconIndex, uint32(px))
	if icon == 0 {
		return nil
	}
	defer winapi.DestroyIcon(icon)
	return iconPixels(icon, px)
}

// iconPixels reads an icon's pixels.
func iconPixels(icon windows.Handle, px int) *image.RGBA {
	var ii winapi.IconInfo
	if !winapi.GetIconInfo(icon, &ii) {
		return nil
	}
	defer func() {
		if ii.Color != 0 {
			winapi.DeleteObject(ii.Color)
		}
		if ii.Mask != 0 {
			winapi.DeleteObject(ii.Mask)
		}
	}()
	if ii.Color == 0 {
		return nil
	}
	dc := winapi.GetDC(0)
	if dc == 0 {
		return nil
	}
	defer winapi.ReleaseDC(0, dc)
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	bgra := img.Pix
	bi := winapi.BitmapInfoHeader{Width: int32(px), Height: -int32(px), Planes: 1, BitCount: 32}
	bi.Size = 40
	if winapi.GetDIBits(dc, ii.Color, uint32(px), unsafe.Pointer(&bgra[0]), &bi) == 0 {
		return nil
	}
	n := px * px
	hasAlpha := false
	for i := range n {
		if bgra[i*4+3] != 0 {
			hasAlpha = true
			break
		}
	}
	if !hasAlpha {
		// An old icon: its mask says what shows.
		mask := make([]byte, n*4)
		mi := winapi.BitmapInfoHeader{Width: int32(px), Height: -int32(px), Planes: 1, BitCount: 32}
		mi.Size = 40
		masked := ii.Mask != 0 && winapi.GetDIBits(dc, ii.Mask, uint32(px), unsafe.Pointer(&mask[0]), &mi) != 0
		for i := range n {
			if masked && mask[i*4] != 0 {
				bgra[i*4+3] = 0
			} else {
				bgra[i*4+3] = 255
			}
		}
	}
	// BGRA to RGBA, premultiplied.
	for i := 0; i < n*4; i += 4 {
		b, g, r, a := uint32(bgra[i]), uint32(bgra[i+1]), uint32(bgra[i+2]), uint32(bgra[i+3])
		bgra[i] = uint8(r * a / 255)
		bgra[i+1] = uint8(g * a / 255)
		bgra[i+2] = uint8(b * a / 255)
	}
	return img
}
