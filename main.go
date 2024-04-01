package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// {"categories":[{
//				"id":69085,
//				"parent_id":0,
//				"type":"department",
//				"name":"Спецпредложения",
//				"slug":"priedlozhieniia",
//				"products_count":12899,
//				"canonical_url":"https://sbermarket.ru/categories/priedlozhieniia",
//				...},...]}

// товарная категория
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"permalink"`
}

// список категорий
type Categories struct {
	All []Category `json:"categories"`
}

//"{"products":[{
//				"id":"1034981",
//				"name":"Пиво Балтика Классическое № 3 светлое 500 мл",
// 				"price":90,
// 				"original_price":90,
// 				"discount":0,
// 				"image_urls\":[\"https://imgproxy.sbermarket.ru/imgproxy/width-auto/czM6Ly9jb250ZW50LWltYWdlcy1wcm9kL3Byb2R1Y3RzLzI3NzEyMzM1L29yaWdpbmFsLzEvMjAyMy0xMC0zMFQxNCUzQTM2JTNBMTIuMjA2MDE3JTJCMDAlM0EwMC8yNzcxMjMzNV8xLmpwZw==.jpg\", \"https://imgproxy.sbermarket.ru/imgproxy/width-auto/czM6Ly9jb250ZW50LWltYWdlcy1wcm9kL3Byb2R1Y3RzLzI3NzEyMzM1L29yaWdpbmFsLzIvMjAyMy0xMC0zMFQxNCUzQTM2JTNBMTIuNDU1NzEyJTJCMDAlM0EwMC8yNzcxMjMzNV8yLmpwZw==.jpg\"], \"requirements\":[{\"type\":\"alcohol\", \"title\":\"Алкоголь\"}], \"slug\":\"pivo-baltika-klassicheskoe-3-svetloe-500-ml-687f338\", \"max_select_quantity\":999, \"canonical_url\":\"https://sbermarket.ru/products/27712335-pivo-baltika-klassicheskoe-3-svetloe-500-ml-687f338\", \"vat_info\":null, \"bmpl_info\":{}, \"max_per_order\":10, \"ads_meta\":null, \"with_options\":false, \"is_beneficial\":false}, {\"id\":\"1094566\", \"sku\":\"13052\", \"retailer_sku\":\"1094566\", \"available\":true, \"legacy_offer_id\":28846261512, \"name\":\"Вино Лыхны красное полусладкое 750 мл Абхазия\", \"price\":699, \"original_price\":699, \"discount\":0, \"human_volume\":\"750 мл\", \"volume\":750, \"volume_type\":\"ml\", \"items_per_pack\":1, \"discount_ends_at\":null, \"price_type\":\"per_item\", \"grams_per_unit\":750, \"unit_price\":699, \"original_unit_price\":699, \"promo_badge_ids\":[], \"score\":5, \"labels\":[], \"image_urls\":[\"https://imgproxy.sbermarket.ru/imgproxy/width-auto/czM6Ly9jb250ZW50LWltYWdlcy1wcm9kL3Byb2R1Y3RzLzEzMDUyL29yaWdpb
// 				"canonical_url":"https://sbermarket.ru/products/879201-sosiski-vyazanka-slivushki-varenye-330-g-2cd7aa1"
//				...},...]}

// товар
type Product struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Price         float64  `json:"price"`
	OriginalPrice float64  `json:"original_price"`
	ImageURL      []string `json:"image_urls"`
	URL           string   `json:"canonical_url"`
}

// список товаров
type Products struct {
	All []Product `json:"products"`
}

// краткая информация о магазине
// из запроса по ближайшим магазинам
type briefShopInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	StoreID int    `json:"store_id"`
}

// {"store":{"id":136,
//			"uuid":"a1ea612b-912e-453a-8a67-fa464b4e1433",
//			"name":"METRO, Калининград, Московский проспект",
//			"full_name":"METRO, Калининград, Калининград, Московский проспект,  279",
//			"location":{
//				"id":126,
//				"full_address":"Калининград, Калининград, Московский проспект,  279",
//				...}...}}

// магазин
type Shop struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Location Location `json:"location"`
}

// локация магазина
type Location struct {
	Full_address string `json:"full_address"`
	City         string `json:"city"`
	Street       string `json:"street"`
}

// структура с данными магазина
type Store struct {
	Shop Shop `json:"store"`
}

const (
	// пауза между запросами (мс)
	REQUEST_PAUSE = 1000
	CSV_FILE      = "products.csv"
	PROXY_IP      = "0.0.0.0"
	PROXY_PORT    = "8080"
	PROXY_LOGIN   = "username"
	PROXY_PASS    = "password"
)

type CSVWriter struct {
	mutex     *sync.Mutex
	csvWriter *csv.Writer
}

func (w *CSVWriter) WriteRow(row []string) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	err := w.csvWriter.Write(row)
	return err
}

func (w *CSVWriter) Flush() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.csvWriter.Flush()
}

func main() {
	// краткая информация о магазинах
	var briefShopInfos []briefShopInfo
	// полная информация о магазинах
	Shops := make(map[int]Shop)
	// категории товаров
	ShopCategories := make(map[int]Categories)

	var wg_parse_products sync.WaitGroup

	userAgent := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	lat := "54.740766"
	lon := "20.437832"
	token := "7ba97b6f4049436dab90c789f946ee2f"
	// указать данные прокси
	http_client := http.Client{
		// Transport: &http.Transport{
		// 	Proxy: http.ProxyURL(&url.URL{
		// 		Scheme: "http",
		// 		User:   url.UserPassword(PROXY_LOGIN, PROXY_PASS),
		// 		Host:   fmt.Sprintf("%s:%s", PROXY_IP, PROXY_PORT),
		// 	}),
		// },
	}

	// Создание файла CSV и потокобезопасного обработчика csv
	file, err := os.Create(CSV_FILE)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	w := csv.NewWriter(file)
	w.Comma = ';'
	csv_writer := &CSVWriter{csvWriter: w, mutex: &sync.Mutex{}}
	defer csv_writer.Flush()
	// заголовки csv
	headers := []string{
		"Name",
		"CurrentPrice",
		"FullPrice",
		"Shop",
		"ShopAddr",
		"Category",
		"PreviewImg",
		"FullImg",
		"Url",
	}
	csv_writer.WriteRow(headers)

	// получение краткой информации о ближайших магазинах по api
	http_req, _ := http.NewRequest("GET", fmt.Sprintf(`https://sbermarket.ru/api/gw/v3/stores?lat=%s&lon=%s&include=closest_shipping_options%%2Clabels%%2Cretailer%%2Clabel_store_ids`,
		lat, lon), nil)
	http_req.Header.Set("User-Agent", userAgent)
	http_req.Header.Set("client-token", token)
	resp, err := http_client.Do(http_req)
	if err != nil {
		log.Println("Failed to do request to api/gw/v3/stores: ", err)
	}
	err = json.NewDecoder(resp.Body).Decode(&briefShopInfos)
	// briefShopInfos - массив элемнтов вида:
	// {ID: "6565517f-4216-45e3-b245-7242c6f3b8f7", Name: "ВИКТОРИЯ, Калининград, Проспект Победы, 137", StoreID: 3623}
	if err != nil {
		log.Println("Failed to Unmarshal stores: ", err)
	}

	// получение детальной информации о каждом магазине по api
	for _, shop := range briefShopInfos {
		http_req, _ := http.NewRequest("GET", fmt.Sprintf(`https://sbermarket.ru/api/stores/%d`,
			shop.StoreID), nil)
		http_req.Header.Set("User-Agent", userAgent)
		http_req.Header.Set("client-token", token)
		resp, _ := http_client.Do(http_req)
		json_store := Store{}
		err = json.NewDecoder(resp.Body).Decode(&json_store)

		if err != nil {
			log.Println("Failed to Unmarshal stores: ", err)
		}
		Shops[shop.StoreID] = json_store.Shop
		log.Println("Получена информация о магазине ", Shops[shop.StoreID].Name)

		time.Sleep(REQUEST_PAUSE * time.Millisecond)

		// test := fmt.Sprintf(`https://sbermarket.ru/api/v3/stores/%d/categories`,
		// 	shop.StoreID)

		// получение категорий, относящихся к магазину
		cat_req, _ := http.NewRequest("GET", fmt.Sprintf(`https://sbermarket.ru/api/categories?store_id=%d`,
			shop.StoreID), nil)
		cat_req.Header.Set("User-Agent", userAgent)
		cat_req.Header.Set("client-token", token)
		resp, err = http_client.Do(cat_req)

		json_categories := Categories{}
		err = json.NewDecoder(resp.Body).Decode(&json_categories)
		ShopCategories[shop.StoreID] = json_categories
		time.Sleep(REQUEST_PAUSE * time.Millisecond)
	}

	// для каждой категории получаем список товаров
	for shopID, categories := range ShopCategories {
		for _, category := range categories.All {
			// ограничиваемся двумя страницами
			for page := 1; page < 3; page++ {
				// подготовка запроса
				var payload = []byte(fmt.Sprintf(`{`+
					`"store_id":"%d",`+
					`"page":"%d",`+
					`"per_page":"24",`+
					`"tenant_id":"sbermarket",`+
					`"filter":[`+
					`{"key":"brand","values":[]},`+
					`{"key":"permalinks","values":[]},`+
					`{"key":"discounted","values":[]}],`+
					`"ads_identity":{"ads_promo_identity":{}},`+
					`"category_permalink":"%s"}`,
					shopID, page, category.Slug,
				))

				req, err := http.NewRequest(
					"POST",
					"https://sbermarket.ru/api/web/v1/products",
					bytes.NewBuffer(payload))
				if err != nil {
					log.Println("Failed to create post request: ", err)
				}
				req.Header.Set("Content-Type", "application/json")

				// запрос
				resp, err := http_client.Do(req)
				if err != nil {
					log.Println("Failed send post request: ", err)
				}
				// запуск парсера продуктов из полученного результата
				wg_parse_products.Add(1)
				shop := Shops[shopID]
				go parse_products(resp.Body, &shop, &category, csv_writer, &wg_parse_products)
				// пауза
				time.Sleep(1 * time.Second)
			}
		}
	}
	wg_parse_products.Wait()
	fmt.Println("Поиск товаров завершён.")
}

func parse_products(
	ioProducts io.ReadCloser,
	shop *Shop,
	category *Category,
	writer *CSVWriter,
	wg *sync.WaitGroup) error {

	defer wg.Done()
	defer ioProducts.Close()

	// //check
	// bodyBytes, err := io.ReadAll(ioProducts)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// bodyString := string(bodyBytes)
	// log.Println(bodyString)

	// декодирование тела респонса
	json_products := Products{}
	err := json.NewDecoder(ioProducts).Decode(&json_products)
	if err != nil {
		return err
	}

	// запись каждого товара в csv файл
	for _, product := range json_products.All {
		imagePreviewUrl := ""
		imageFullUrl := ""
		if len(product.ImageURL) > 0 {
			imagePreviewUrl = strings.Replace(product.ImageURL[0], "width-auto", "size-220-220", 1)
			imageFullUrl = strings.Replace(product.ImageURL[0], "width-auto", "size-1646-1646", 1)
		}

		err = writer.WriteRow([]string{
			product.Name,
			fmt.Sprintf("%f", product.Price),
			fmt.Sprintf("%f", product.OriginalPrice),
			shop.Name,
			shop.Location.Full_address,
			category.Name,
			imagePreviewUrl,
			imageFullUrl,
			product.URL,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
