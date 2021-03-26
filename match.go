package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"

	"github.com/omigo/log"
	"gocv.io/x/gocv"
)

func main() {
	http.HandleFunc("/getdistance", cors(getDistance))
	http.ListenAndServe(":8080", nil)
}

func cors(next func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,AccessToken,X-CSRF-Token, Authorization, Token, X-Token")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		//放行所有OPTIONS方法
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		defer func() {
			if e := recover(); e != nil {
				log.Error(e)
			}
		}()
		next(w, r)
	}
}

func getReq(r *http.Request) (req *Req, err error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	//log.Debugf("%s", body)
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func getDistance(w http.ResponseWriter, r *http.Request) {
	req, err := getReq(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	log.JsonIndent(req)

	alpha, block, err := preProcess(req.BlockBase64, req.BlockWidth, req.BlockHeight)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	defer block.Close()

	_, bg, err := preProcess(req.BgBase64, req.BgWidth, req.BgHeight)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	defer bg.Close()

	loc := match(bg, block, alpha)

	fmt.Fprintf(w, `{"distance":%d}`, loc.X)
}

type Req struct {
	BgBase64    string `json:"bg_base64"`
	BgWidth     int    `json:"bg_width"`
	BgHeight    int    `json:"bg_height"`
	BlockBase64 string `json:"block_base64"`
	BlockWidth  int    `json:"block_width"`
	BlockHeight int    `json:"block_height"`
}

func decode(b64img string) []byte {
	i := strings.IndexByte(b64img, ',')
	if i == -1 {
		log.Error(b64img)
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(b64img[i+1:])
	if err != nil {
		log.Error(err, b64img[i+1:])
		return nil
	}
	return b
}

func readBase64Image(b64Image string) (gocv.Mat, error) {
	origin, err := gocv.IMDecode(decode(b64Image), gocv.IMReadUnchanged)
	if err != nil {
		return gocv.Mat{}, err
	}
	return origin, nil
}

func resize(origin gocv.Mat, cols, rows int) gocv.Mat {
	resized := gocv.NewMatWithSize(cols, rows, origin.Type())
	gocv.Resize(origin, &resized, image.Pt(cols, rows), 0, 0, gocv.InterpolationNearestNeighbor)
	return resized
}

func gray(origin gocv.Mat) gocv.Mat {
	grayed := gocv.NewMat()
	gocv.CvtColor(origin, &grayed, gocv.ColorBGRToGray)
	return grayed
}

func threshold(origin gocv.Mat) gocv.Mat {
	thresholdBG := gocv.NewMat()
	gocv.Threshold(origin, &thresholdBG, 100, 255, gocv.ThresholdBinaryInv)
	return thresholdBG
}

func match(bg, block, mask gocv.Mat) image.Point {
	result := gocv.NewMatWithSize(
		bg.Rows()-block.Rows()+1,
		bg.Cols()-block.Cols()+1,
		gocv.MatTypeCV32FC1)
	defer result.Close()

	gocv.MatchTemplate(bg, block, &result, gocv.TmSqdiff, mask)
	gocv.Normalize(result, &result, 0, 1, gocv.NormMinMax)

	_, _, _, maxLoc := gocv.MinMaxLoc(result)

	return maxLoc
}

func preProcess(b64Image string, width, heigh int) (alpha, processed gocv.Mat, err error) {
	origin, err := readBase64Image(b64Image)
	if err != nil {
		return gocv.Mat{}, gocv.Mat{}, err
	}
	//defer origin.Close()

	resized := resize(origin, width, heigh)
	grayed := gray(resized)
	//threshold := threshold(grayed)
	//defer resized.Close()
	//defer grayed.Close()
	//defer threshold.Close()

	log.Debug(origin.Cols(), origin.Rows(), resized.Cols(), resized.Rows())

	if resized.Channels() == 4 {
		return gocv.Split(resized)[3], grayed, nil
	}

	return gocv.Mat{}, grayed, nil
}
