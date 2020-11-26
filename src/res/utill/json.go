package utill

import (
	"encoding/json"
)

func ToJSON(obj interface{}) string {
	data, err := json.Marshal(obj)
	if err != nil {
		Stde(err)
		return "error"
	}
	return string(data)
}
