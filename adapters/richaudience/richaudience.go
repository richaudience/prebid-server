package richaudience

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mxmCherry/openrtb/v15/openrtb2"
	"github.com/prebid/prebid-server/adapters"
	"github.com/prebid/prebid-server/config"
	"github.com/prebid/prebid-server/errortypes"
	"github.com/prebid/prebid-server/openrtb_ext"
	"github.com/prebid/prebid-server/pbs"
)

type RichaudienceAdapter struct {
	endpoint    string
	consent     string
	gdprApplies int
}

// Builder builds a new instance of the Foo adapter for the given bidder with the given config.
func Builder(bidderName openrtb_ext.BidderName, config config.Adapter) (adapters.Bidder, error) {
	bidder := &RichaudienceAdapter{
		endpoint: config.Endpoint,
	}

	fmt.Printf(bidder.endpoint)

	return bidder, nil
}

func (a *RichaudienceAdapter) Call(ctx context.Context, req *pbs.PBSRequest, bidder *pbs.PBSBidder) (pbs.PBSBidSlice, error) {

	//a.consent, _, _ = req.Cookie.GetUID("euconsent")

	//a.gdprApplies := request.ParseGDPR()

	a.consent = req.ParseConsent()

	fmt.Printf("ENTRA")
	fmt.Printf(a.consent)

	return pbs.PBSBidSlice{}, nil
}

func (a *RichaudienceAdapter) MakeRequests(request *openrtb2.BidRequest, requestInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {

	fmt.Printf("MAKE")

	richaudienceRequest := *request
	raiHeaders := http.Header{}
	secure := int8(0)
	lmt := int8(0)
	dnt := int8(0)
	resImps := make([]openrtb2.Imp, 0, len(request.Imp))

	raiHeaders.Set("Content-Type", "application/json;charset=utf-8")
	raiHeaders.Set("Accept", "application/json")

	//Imp: Id, tagId, Secure, Bidfloor, Bidfloorcur, Banner
	//Banner: Id, W, H, Format
	//Format: W, H

	if request.Site != nil && request.Site.Page != "" {
		pageURL, err := url.Parse(request.Site.Page)
		if err == nil && pageURL.Scheme == "https" {
			secure = int8(1)
		}
	}

	for _, imp := range request.Imp {
		raiExt, err := parseImpExt(&imp)
		if err != nil {
			fmt.Printf("%s", err)
		}

		addParametersImp(&imp, &secure, raiExt)

		if imp.Banner == nil {
			fmt.Printf("Banner Object not found")
		} else {
			raiBanner := *imp.Banner
			if raiBanner.W == nil && raiBanner.H == nil {
				if len(raiBanner.Format) == 0 {
					fmt.Printf("Format Object not found")
				} else {
					raiBanner.ID = "testId-Banner"
					imp.Banner = &raiBanner
				}
			}
		}

		resImps = append(resImps, imp)
	}

	richaudienceRequest.Imp = resImps

	//Site: Page,Domain - ID
	if request.Site.Domain == "" {
		raiUrl := strings.Split(request.Site.Page, "//")
		richaudienceRequest.Site.Domain = strings.Split(raiUrl[1], "/")[0]
	} else {
		richaudienceRequest.Site.Domain = request.Site.Domain
	}

	//Device: UA - IP, LMT, DNT
	if request.Device.IP == "" {
		richaudienceRequest.Device.IP = "11.222.33.44"
	} else {
		richaudienceRequest.Device.IP = request.Device.IP
	}

	richaudienceRequest.Device.DNT = &dnt
	richaudienceRequest.Device.Lmt = &lmt

	//Regs: GDPR

	richaudienceRequest.Device.IP = a.consent

	//User: Buyeruid, consent

	req, err := json.Marshal(richaudienceRequest)
	if err != nil {
		fmt.Printf("%s", err)
	}

	requestData := &adapters.RequestData{
		Method:  "POST",
		Uri:     a.endpoint,
		Body:    req,
		Headers: raiHeaders,
	}

	return []*adapters.RequestData{requestData}, nil
}

func (a *RichaudienceAdapter) MakeBids(request *openrtb2.BidRequest, requestData *adapters.RequestData, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if responseData.StatusCode == http.StatusBadRequest {
		err := &errortypes.BadInput{
			Message: "Unexpected status code: 400. Bad request from publisher. Run with request.debug = 1 for more info.",
		}
		return nil, []error{err}
	}

	if responseData.StatusCode != http.StatusOK {
		err := &errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info.", responseData.StatusCode),
		}
		return nil, []error{err}
	}

	var response openrtb2.BidResponse
	if err := json.Unmarshal(responseData.Body, &response); err != nil {
		return nil, []error{err}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(len(request.Imp))
	bidResponse.Currency = response.Cur
	for _, seatBid := range response.SeatBid {
		for _, bid := range seatBid.Bid {
			b := &adapters.TypedBid{
				Bid:     &bid,
				BidType: "banner",
			}
			bidResponse.Bids = append(bidResponse.Bids, b)
		}
	}
	return bidResponse, nil
}

//utils
func parseImpExt(imp *openrtb2.Imp) (*openrtb_ext.ExtImpRichaudience, error) {
	var bidderExt adapters.ExtImpBidder
	if err := json.Unmarshal(imp.Ext, &bidderExt); err != nil {
		return nil, &errortypes.BadInput{
			Message: fmt.Sprintf("not found parameters ext in ImpID : %s", imp.ID),
		}
	}

	var richaudienceExt openrtb_ext.ExtImpRichaudience
	if err := json.Unmarshal(bidderExt.Bidder, &richaudienceExt); err != nil {
		return nil, &errortypes.BadInput{
			Message: fmt.Sprintf("invalid parameters ext in ImpID: %s", imp.ID),
		}
	}

	return &richaudienceExt, nil
}

func addParametersImp(imp *openrtb2.Imp, secure *int8, richaudienceExt *openrtb_ext.ExtImpRichaudience) {

	imp.TagID = richaudienceExt.Pid
	imp.Secure = secure

	if richaudienceExt.BidFloor <= 0 {
		imp.BidFloor = 0.00001
	} else {
		imp.BidFloor = richaudienceExt.BidFloor
	}

	if richaudienceExt.BidFloorCur == "" {
		imp.BidFloorCur = "USD"
	} else {
		imp.BidFloorCur = richaudienceExt.BidFloorCur
	}

	imp.Banner.ID = "ID-Test"

}
