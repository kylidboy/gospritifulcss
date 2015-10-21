package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

type imgDecoder func(io.Reader) (image.Image, error)

type myImage struct {
	img   image.Image
	name  string
	point image.Point
}

type myImageSlice []myImage

var (
	src        = flag.String("src", "./", "source dir where all the images located")
	out        = flag.String("out", "./", "output dir")
	name       = flag.String("name", "sprite", "name for the output without extension")
	extensions = flag.String("extensions", "jpg,png", "file extensions that will be included, e.g. jpg,png,gif")
	marginP    = flag.Int("margin", 4, "margin between each component, also between the new image borders")

	margin         int
	filenameFilter *regexp.Regexp
	myImages       myImageSlice

	wg            *sync.WaitGroup = new(sync.WaitGroup)
	imgBufferLock *sync.Mutex     = new(sync.Mutex)
)

func init() {
	flag.Parse()

	exts := strings.Split(*extensions, ",")
	filenameFilter = regexp.MustCompile(".*\\.(?i:" + strings.Join(exts, "|") + ")")
	margin = *marginP
}

func getImagesAbsPath(root string, filter *regexp.Regexp) (imagenames []string) {
	absPath, err := filepath.Abs(root)
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	filenames, err := filepath.Glob(filepath.Join(absPath, "*"))
	if err != nil {
		fmt.Println(err)
		os.Exit(-2)
	}

	for _, x := range filenames {
		if filter.MatchString(x) {
			imagenames = append(imagenames, x)
		}
	}

	return
}

func readImage(p string) {
	defer wg.Done()

	handler, err := os.Open(p)
	if err != nil {
		fmt.Println(err)
		runtime.Goexit()
	}

	var decoder imgDecoder
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		decoder = png.Decode
	case ".jpg":
		decoder = jpeg.Decode
	case ".gif":
		decoder = gif.Decode
	}

	img, err := decoder(handler)

	if err != nil {
		fmt.Println(err)
		runtime.Goexit()
	}

	imgBufferLock.Lock()
	myImages = append(myImages, myImage{
		img:  img,
		name: filepath.Base(p),
	})
	imgBufferLock.Unlock()
}

func getProductSize() image.Rectangle {
	var w int = 0
	var h int = 0

	for _, i := range myImages {
		rect := i.img.Bounds()
		w = int(math.Max(float64(w), float64(rect.Dx())))
		h += rect.Dy()
	}

	h += margin * (len(myImages) + 1)
	w += 2 * margin

	return image.Rect(0, 0, w, h)
}

func fillInSprite(rect image.Rectangle) *image.NRGBA {
	var nrgba *image.NRGBA = image.NewNRGBA(rect)
	var left int = margin
	var top int = margin

	for idx, i := range myImages {
		wg.Add(1)

		go (func(img image.Image, left int, top int) {
			defer wg.Done()
			draw.Draw(nrgba, image.Rect(left, top, left+img.Bounds().Dx(), top+img.Bounds().Dy()), img, image.ZP, draw.Src)
		})(i.img, left, top)

		myImages[idx].point = image.Pt(left, top)
		top += i.img.Bounds().Dy() + margin
	}

	wg.Wait()

	return nrgba
}

func writeSprite(nrgba *image.NRGBA) {
	absOut, err := filepath.Abs(*out)
	if err != nil {
		fmt.Println(err)
		os.Exit(-127)
	}

	dirH, err := os.Stat(absOut)
	if err != nil {
		if os.IsNotExist(err) {
			e := os.MkdirAll(absOut, 0775)
			if e != nil {
				fmt.Println(e)
				os.Exit(-1)
			}
		} else {
			fmt.Println(err)
			os.Exit(-128)
		}
	} else if !dirH.IsDir() {
		fmt.Println("output should be a directory!")
		os.Exit(-1)
	}

	spriteFilename := *name + ".png"
	spriteFile, err := os.Create(filepath.Join(absOut, spriteFilename))
	png.Encode(spriteFile, nrgba)

	generateDemo(filepath.Join(absOut, *name+".html"), spriteFilename)
}

func generateDemo(demoPathname string, spriteFilename string) {
	var className string
	divTags := make([]string, 0, len(myImages))
	cssBlocks := make([]string, 0, len(myImages))

	cssBlocks = append(cssBlocks, fmt.Sprintf(`.icon { background: url("/%s") no-repeat; }`, spriteFilename))

	for _, i := range myImages {
		className = "icon-" + strings.Replace(i.name, ".", "-", -1)
		divTags = append(divTags, fmt.Sprintf(`<div class="icon %s"></div>`, className))
		cssBlocks = append(cssBlocks, fmt.Sprintf(".%s { background-position: left %dpx top %dpx; width:%dpx; height:%dpx;}", className, -i.point.X, -i.point.Y, i.img.Bounds().Dx(), i.img.Bounds().Dy()))
	}

	htmlTemplate := `<html><head><style type="text/css">%s</style></head><body>%s</body></html>`
	htmlHandler, err := os.Create(demoPathname)
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	htmlHandler.WriteString(fmt.Sprintf(htmlTemplate, strings.Join(cssBlocks, ""), strings.Join(divTags, "")))
	htmlHandler.Sync()
}

func main() {
	imagenames := getImagesAbsPath(*src, filenameFilter)

	myImages = make(myImageSlice, 0, len(imagenames))

	for _, i := range imagenames {
		wg.Add(1)
		go readImage(i)
	}

	wg.Wait()

	writeSprite(fillInSprite(getProductSize()))
}
