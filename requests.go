package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

var apiEndpoints = map[string]string{
	"blocks_endpoint":              "/cosmos/base/tendermint/v1beta1/blocks/%d",
	"latest_block_endpoint":        "/blocks/latest",
	"txs_endpoint":                 "/cosmos/tx/v1beta1/txs/%s",
	"txs_by_block_height_endpoint": "/cosmos/tx/v1beta1/txs?events=tx.height=%d&pagination.limit=100&order_by=ORDER_BY_UNSPECIFIED",
}

//GetBlockByHeight makes a request to the Cosmos REST API to get a block by height
func GetBlockByHeight(host string, height uint64) (GetBlockByHeightResponse, error) {

	var result GetBlockByHeightResponse

	requestEndpoint := fmt.Sprintf(apiEndpoints["blocks_endpoint"], height)

	resp, err := http.Get(fmt.Sprintf("%s%s", host, requestEndpoint))

	if err != nil {
		return result, err
	}

	defer resp.Body.Close()

	err = checkResponseErrorCode(requestEndpoint, resp)
	if err != nil {
		return result, err
	}

	//TODO: need to check resp.Status

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}

	json.Unmarshal(body, &result)

	return result, nil
}

//GetTxsByBlockHeight makes a request to the Cosmos REST API and returns all the transactions for a specific block
func GetTxsByBlockHeight(host string, height uint64) (GetTxByBlockHeightResponse, error) {

	var result GetTxByBlockHeightResponse

	requestEndpoint := fmt.Sprintf(apiEndpoints["txs_by_block_height_endpoint"], height)

	resp, err := http.Get(fmt.Sprintf("%s%s", host, requestEndpoint))

	if err != nil {
		return result, err
	}

	defer resp.Body.Close()

	err = checkResponseErrorCode(requestEndpoint, resp)

	if err != nil {
		return result, err
	}

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return result, err
	}

	err = json.Unmarshal(body, &result)

	if err != nil {
		return result, err
	}

	return result, nil
}

func GetLatestBlock(host string) (GetLatestBlockResponse, error) {

	var result GetLatestBlockResponse

	requestEndpoint := apiEndpoints["latest_block_endpoint"]

	resp, err := http.Get(fmt.Sprintf("%s%s", host, requestEndpoint))

	if err != nil {
		return result, err
	}

	defer resp.Body.Close()

	err = checkResponseErrorCode(requestEndpoint, resp)

	if err != nil {
		return result, err
	}

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return result, err
	}

	err = json.Unmarshal(body, &result)

	if err != nil {
		return result, err
	}

	return result, nil
}

func checkResponseErrorCode(requestEndpoint string, resp *http.Response) error {

	if resp.StatusCode != 200 {
		fmt.Println("Error getting response")
		body, _ := ioutil.ReadAll(resp.Body)
		errorString := fmt.Sprintf("Error getting response for endpoint %s: Status %s Body %s", requestEndpoint, resp.Status, body)

		err := errors.New(errorString)

		return err
	}

	return nil

}
