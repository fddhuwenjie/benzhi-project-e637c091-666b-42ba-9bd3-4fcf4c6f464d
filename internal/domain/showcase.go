package domain

type Showcase struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	GalleryZone          string          `json:"gallery_zone"`
	CollectionLevel      CollectionLevel `json:"collection_level"`
	SensorIDs            []string        `json:"sensor_ids"`
	TargetTemperatureMin float64         `json:"target_temperature_min"`
	TargetTemperatureMax float64         `json:"target_temperature_max"`
	TargetHumidityMin    float64         `json:"target_humidity_min"`
	TargetHumidityMax    float64         `json:"target_humidity_max"`
	Active               bool            `json:"active"`
}
