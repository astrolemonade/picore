package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/schollz/logger"
	"github.com/youpy/go-wav"
)

var flagFolder string
var flagFolderOut string
var flagLimit int
var flagBPM float64
var flagSR float64

func init() {
	flag.StringVar(&flagFolder, "folder-in", "flacs", "folder for finding audio")
	flag.StringVar(&flagFolderOut, "folder-out", "converted", "folder for placing converted files")
	flag.IntVar(&flagLimit, "limit", 5, "limit number of samples")
	flag.Float64Var(&flagBPM, "bpm", 180, "bpm to set to")
	flag.Float64Var(&flagSR, "sr", 19200, "sample rate to set to")
}

func main() {
	flag.Parse()
	log.SetLevel("trace")
	files, err := getFiles()
	if err != nil {
		return
	}
	err = convertFiles(files)
	if err != nil {
		return
	}
	err = audio2h(files)
	if err != nil {
		return
	}
}

func audio2h(files []File) (err error) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })

	var sb strings.Builder
	limit := len(files)
	if limit > flagLimit {
		limit = flagLimit
	}
	sb.WriteString("#include <pico/platform.h>\n\n")
	sb.WriteString(fmt.Sprintf("#define NUM_SAMPLES %d\n", limit))
	sb.WriteString(fmt.Sprintf("#define SAMPLES_PER_BEAT %d\n", int(math.Round(60/flagBPM*flagSR/2))))
	numRetrigs := 5
	startRetrig := math.Round(60 / flagBPM * flagSR / 2)
	retrigs := make([]string, int(numRetrigs))
	for i := 0; i < numRetrigs; i++ {
		retrigs[i] = fmt.Sprintf("%d", int(startRetrig/math.Pow(2, float64(i))))
	}
	retrigs = append([]string{
		fmt.Sprint(startRetrig * 4),
		fmt.Sprint(startRetrig * 3),
		fmt.Sprint(startRetrig * 2),
	}, retrigs...)
	sb.WriteString(fmt.Sprintf("#define NUM_RETRIGS %d\n", len(retrigs)))
	sb.WriteString("const uint16_t retrigs[] = { 12800, 9600, 6400, 4267, 3200, 2133, 1600, 800, 400, 200 };") // TODO fix this
	for i, f := range files {
		var ints []int
		ints, err = convertWavToInts(f.Converted)
		if err != nil {
			log.Error(err)
			return
		}
		log.Tracef("%s: %d", f.Converted, len(ints))
		sb.WriteString("\n\n// " + filepath.Base(f.Pathname) + "\n")
		sb.WriteString(fmt.Sprintf("#define RAW_%d_BEATS %d\n", i, int(f.Beats)*2))
		sb.WriteString(fmt.Sprintf("#define RAW_%d_SAMPLES %d\n", i, len(ints)))
		sb.WriteString(fmt.Sprintf("const unsigned char __in_flash() raw_%d[] = {\n", i))
		sb.WriteString(printInts(ints))
		sb.WriteString("\n};\n\n\n")

		if i == limit {
			break
		}
	}

	sb.WriteString("\n\n")

	sb.WriteString("char raw_val(int s, int i) {\n")
	for i := range files {
		sb.WriteString(fmt.Sprintf("\tif (s==%d) return raw_%d[i];\n", i, i))
		if i == limit {
			break
		}
	}
	sb.WriteString("return raw_0[i];\n}\n\n")

	sb.WriteString("unsigned int raw_len(int s) {\n")
	for i := range files {
		sb.WriteString(fmt.Sprintf("\tif (s==%d) return RAW_%d_SAMPLES;\n", i, i))
		if i == limit {
			break
		}
	}
	sb.WriteString("return RAW_0_SAMPLES;\n}\n\n")

	sb.WriteString("unsigned int raw_beats(int s) {\n")
	for i := range files {
		sb.WriteString(fmt.Sprintf("\tif (s==%d) return RAW_%d_BEATS;\n", i, i))
		if i == limit {
			break
		}
	}
	sb.WriteString("return 1;\n}\n\n")

	f, err := os.Create("../audio2h.h")
	f.WriteString(sb.String())
	f.Close()
	return
}

func printInts(ints []int) (s string) {
	var sb strings.Builder
	sb.WriteString("\t")
	for i, v := range ints {
		sb.WriteString(fmt.Sprintf("0x%02x", v))
		if i < len(ints)-1 {
			sb.WriteString(", ")
		}
		if i > 0 && i%20 == 0 {
			sb.WriteString("\n\t")
		}

	}
	s = sb.String()
	return

}

type File struct {
	Pathname  string
	Converted string
	Beats     float64
	BPM       float64
}

func getFiles() (files []File, err error) {
	fnames := []string{}
	err = filepath.Walk(".",
		func(pathname string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			ext := filepath.Ext(pathname)
			if ext == ".flac" || ext == ".wav" || ext == ".mp3" || ext == ".aif" || ext == ".ogg" {
				fnames = append(fnames, pathname)
			}
			return nil
		})
	if err != nil {
		return
	}
	log.Infof("found %d files", len(fnames))
	rBeats, _ := regexp.Compile(`\w+[beats](\d+)\w+`)
	rBPM, _ := regexp.Compile(`\w+[bpm]([0-9]+).`)
	files = make([]File, len(fnames))
	i := 0
	for _, fname := range fnames {
		files[i].Pathname = fname
		m := rBeats.FindStringSubmatch(fname)
		if len(m) < 2 {
			continue
		}
		files[i].Converted = path.Join(flagFolderOut, filepath.Base(fname)+".wav")
		files[i].Beats, _ = strconv.ParseFloat(rBeats.FindStringSubmatch(fname)[1], 64)
		files[i].BPM, _ = strconv.ParseFloat(rBPM.FindStringSubmatch(fname)[1], 64)
		log.Tracef("0: %+v", files[i])
		i++
	}
	files = files[:i]
	return
}

func convertFiles(files []File) (err error) {
	os.MkdirAll(flagFolderOut, os.ModePerm)
	i := 0
	for _, f := range files {
		cmd := fmt.Sprintf("sox %s -r %d -c 1 -b 8 %s speed %2.6f lowpass %d norm gain -3", f.Pathname, int(flagSR), files[i].Converted, flagBPM/f.BPM, int(flagSR))
		err = ex(cmd)
		if err == nil {
			i++
		}
	}
	err = nil
	return
}

func ex(c string) (err error) {
	log.Trace(c)
	cs := strings.Fields(c)
	cmd := exec.Command(cs[0], cs[1:]...)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("cmd failed: %s\n%s", c, stdoutStderr)
	}
	return
}

func convertWavToInts(fname string) (vals []int, err error) {
	file, err := os.Open(fname)
	if err != nil {
		return
	}
	reader := wav.NewReader(file)
	n := 0
	vals = make([]int, 1000000)
	for {
		samples, err := reader.ReadSamples()

		for _, sample := range samples {
			v := reader.IntValue(sample, 0)
			vals[n] = v
			n++
		}

		if err == io.EOF {
			break
		}
	}
	err = file.Close()

	vals = vals[:n]
	return
}

func numSamples(fname string) (samples int, sampleRate int, err error) {
	r, _ := regexp.Compile(`(\d+) samples`)
	r2, _ := regexp.Compile(`(\d+)`)
	r3, _ := regexp.Compile(`Sample Rate\s+:\s+(\d+)`)

	cmd := exec.Command("sox", "--i", fname)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return
	}
	log.Trace(string(output))
	match := r.FindString(string(output))
	match2 := r2.FindString(match)
	samples, err = strconv.Atoi(match2)

	match = r3.FindString(string(output))
	match2 = r2.FindString(match)
	sampleRate, err = strconv.Atoi(match2)

	return
}
