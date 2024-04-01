package with_selenium

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

// cd selenium && go run init.go --alsologtostderr  --download_browsers --download_latest

// товар
type Product struct {
	Shop         string
	ShopAddr     string
	Category     *Category
	Url          string
	Name         string
	PreviewImg   string
	FullImg      string
	CurrentPrice string
	FullPrice    string
}

// список тоаров
type Products struct {
	All []Product
	sync.Mutex
}

// добавить товар в список
func (p *Products) Add(newProduct Product) error {
	p.Lock()
	defer p.Unlock()
	p.All = append(p.All, newProduct)
	return nil
}

// товарная категория
type Category struct {
	Shop       *Shop
	Parent     *Category
	Url        string
	Name       string
	HasContent bool
}

// список категорий
type Categories struct {
	All []Category
	sync.Mutex
}

// добавить категорию
func (c *Categories) Add(newCat Category) error {
	c.Lock()
	c.All = append(c.All, newCat)
	c.Unlock()
	return nil
}

// магазин (для получения по api)
type Shop struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Location Location `json:"location"`
}

// локация магазина (для получения по api)
type Location struct {
	Full_address string `json:"full_address"`
	City         string `json:"city"`
	Street       string `json:"street"`
}

// структура с данными магазина (для получения по api)
type Store struct {
	Shop Shop `json:"store"`
}

const (
	// duration in seconds for one request
	// not implemented yet
	REQUEST_FREQ = 1
	DEBUG        = true
)

func main() {
	// для ограничения частоты запросов (не реализовано на данный момент)
	lastRequest := LastRequest{}
	// категории товаров
	categories := Categories{}
	// товары
	products := Products{}
	//proxyServerURL := "213.157.6.50"
	userAgent := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	start_url := "https://sbermarket.ru"
	fileName := "products.csv"
	// переменную location следует изменить для получения данных из других магазинов
	location := "площадь Победы, 1, Калининград"
	// lat := 54.740766
	// lon := 20.437832
	token := "7ba97b6f4049436dab90c789f946ee2f"
	// указать данные прокси
	http_client := http.Client{
		// Transport: &http.Transport{
		// 	Proxy: http.ProxyURL(&url.URL{
		// 		Scheme: "http",
		// 		User:   url.UserPassword("login", "password"),
		// 		Host:   "IP:PORT",
		// 	}),
		// },
	}
	var wg_find_products sync.WaitGroup

	// Фукция для парсинга товаров из ресурсов страницы в виде string
	// source - вся страница в виде string
	// category - категория, к которой будут относиться найдеый товары
	find_products := func(source string, category Category) {
		defer wg_find_products.Done()

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(source))
		if err != nil {
			log.Fatal(err)
		}
		doc.Find("div.ProductCard_root__K6IZK").Each(func(i int, s *goquery.Selection) {
			// url товара
			part_url, _ := s.Find("a").Attr("href")
			url := start_url + part_url
			// название товара
			name := s.Find("h3.ProductCard_title__iNsaD").Text()
			// маленькая картинка
			previewImg, _ := s.Find("img.ProductCard_image__3jwTC").Attr("src")
			// большая картинка
			// по наблюдениям, большая картинка отличается от маленькой только другим
			// разрешением в ссылке, поэтому fullImg создаётся на основании previewImg
			// иначе придётся нажимать на кнопку открытия детальной информации о товаре
			fullImg := strings.Replace(previewImg, "size-220-220", "size-1646-1646", 1)
			// получение и очистка цены / цены со скидкой
			str_price := s.Find("div.ProductCardPrice_price__Kv7Q7").Text()
			prefix := s.Find("div.ProductCardPrice_price__Kv7Q7").Find("span").Text()
			price, _ := ClearPrice(str_price, prefix)
			// получение и очистка оригинальной цены при наличии скидки
			str_fullPrice := s.Find("div.ProductCardPrice_originalPrice__z36Di").Text()
			prefix = s.Find("div.ProductCardPrice_originalPrice__z36Di").Find("span").Text()
			fullPrice, _ := ClearPrice(str_fullPrice, prefix)
			// добавление найденного товара в общий список товаров
			products.Add(Product{
				Url:          url,
				Shop:         category.Shop.Name,
				ShopAddr:     category.Shop.Location.Full_address,
				Category:     &category,
				Name:         name,
				PreviewImg:   previewImg,
				FullImg:      fullImg,
				CurrentPrice: price,
				FullPrice:    fullPrice,
			})
			log.Printf("Найден товар %s в категории %s магазина %s\n", name, category.Name, category.Shop.Name)
		})
	}

	selenium.SetDebug(DEBUG)
	// получение свободного порта для selenium
	port, err := pickUnusedPort()
	if err != nil {
		log.Fatal("pickUnusedPort() returned error:", err)
	}

	service, err := selenium.NewChromeDriverService("./chromedriver.v123", port)
	if err != nil {
		log.Fatal("Error:", err)
	}
	defer service.Stop()

	// configure the browser options
	caps := selenium.Capabilities{"browserName": "chrome"}
	caps.AddChrome(chrome.Capabilities{Args: []string{
		"--headless=new",            // comment out this line for testing
		"--no-sandbox",              // Bypass OS security model
		"--disable-dev-shm-usage",   // overcome limited resource problems
		"--disable-gpu",             // applicable to windows os only
		"--disable-extensions",      // disabling extensions
		"--disable-infobars",        // disabling infobars
		"--User-Agent=" + userAgent, // user agent
		"--disable-blink-features=AutomationControlled",
		"--useAutomationExtension=false",
		//"--proxy-server=" + proxyServerURL, // proxysbe
	},
		Path: "./chrome/chrome",
		ExcludeSwitches: []string{
			"load-extension",
			"enable-automation"},
	})

	// create a new remote client with the specified options
	webdriver, err := selenium.NewRemote(caps, fmt.Sprintf("http://127.0.0.1:%d/wd/hub", port))
	if err != nil {
		log.Fatal("Error:", err)
	}

	// webdriver.ExecuteScript()

	// maximize the current window to avoid responsive rendering
	err = webdriver.MaximizeWindow("")
	if err != nil {
		log.Fatal("Error:", err)
	}

	defer webdriver.Quit()

	webdriver.ExecuteScript("window.key = \"blahblah\";", nil)

	// Navigate to the simple playground interface.
	if err := webdriver.Get(start_url); err != nil {
		log.Fatalln("Failed to get page! Error:", err)
	}

	// ByID, ByXPATH, ByLinkText, ByPartialLinkText, ByName, ByTagName, ByClassName, ByCSSSelector
	// кнопка "Показать все магазины"
	btnAllRetailers, err := webdriver.FindElement(selenium.ByXPATH, "//button[contains(text(),'Показать всех')]")
	if err != nil {
		panic(err)
	}

	// если кнопка не нажимается - закрыть модальные окна
	err = btnAllRetailers.Click()
	if err != nil { // ModalWrapper_root
		modalDialog, err := webdriver.FindElement(selenium.ByXPATH, `//div[contains(@class,"FirstPromoModal")]`)
		if err == nil {
			btnClose, err := modalDialog.FindElement(selenium.ByXPATH, "//button[contains(@class, 'Modal_closeButton')]")
			if err == nil {
				err = btnClose.Click()
				if err != nil {
					panic(err)
				}
			}
		}
		btnOkCookies, err := webdriver.FindElement(selenium.ByXPATH, "//div[contains(@class,'CookiesConcent')]/button")
		if err == nil {
			btnOkCookies.Click()
		}

		err = btnAllRetailers.Click()
		if err != nil {
			log.Println(err)
		}
	}

	// откроется окно для выбора местоположения
	err = WaitForElement(webdriver, "//div[contains(@class, 'DeliveryMap_search')]", 10*time.Second)
	if err != nil {
		log.Fatal("Error:", err)
	}
	// поиск поля для заполнения местоположения
	searchLocation, err := webdriver.FindElement(selenium.ByXPATH, "//div[contains(@class,'DeliveryMap_search')]")
	if err != nil {
		panic(err)
	}
	// активация поля
	err = searchLocation.Click()
	if err != nil {
		log.Println(err)
	}
	// заполнение поля текстом из переменной location
	err = printText(webdriver, location)
	if err != nil {
		log.Println(err)
	}

	// выбор адреса из выпадающего списка
	drop, err := searchLocation.FindElement(selenium.ByXPATH,
		"//div[contains(@class, 'SearchSelectForMap_dropdown')]/div/child::node()[1]")
	if err != nil {
		log.Println("Отсутствует выпадающий список", err)
	}
	text, _ := drop.Text()
	if text != location {
		log.Panic("Не найдена указанная локация!")
	}
	err = drop.Click()
	if err != nil {
		log.Panic("Ошибка выбора метсоположения", err)
	}

	// ожидание активации кнопки сохранения
	err = webdriver.WaitWithTimeout(func(driver selenium.WebDriver) (bool, error) {
		btn, _ := driver.FindElement(selenium.ByXPATH, "//div[contains(@class,'DeliveryMap_search')]//button")

		if btn != nil {
			return btn.IsEnabled()
		}
		return false, nil
	}, 10*time.Second)
	if err != nil {
		log.Fatal("Error:", err)
	}

	// поиск кнопки перебором с TabKey и нажатие
	// в Selenium кнопка другим сопосбом не нажимается
	for i := 0; i < 10; i++ {
		webdriver.KeyDown(selenium.TabKey)
		el, err := webdriver.ActiveElement()
		if err != nil {
			log.Println(el.Text())
		}
		text, _ := el.Text()
		if text == "Сохранить" || text == "Ok" {
			webdriver.KeyDown(selenium.EnterKey)
			break
		}
	}

	// получение ссылок на магазины
	WaitForElement(webdriver, "//div[contains(@class, 'Stores_storeWrapper')]//a", 10*time.Second)
	stores_urls := []string{}
	aStores, _ := webdriver.FindElements(
		selenium.ByXPATH,
		"//div[contains(@class, 'Stores_storeWrapper')]//a[contains(@class, 'Stores_store')]")
	for _, aStore := range aStores {
		store_url, _ := aStore.GetAttribute("href")
		stores_urls = append(stores_urls, store_url)
	}

	// для каждого магазина происходит поиск категорий и добавление найденный категорий в общий список
	for _, store_root := range stores_urls {

		// получение id магазина из url
		str_store_id := strings.Split(store_root, "/stores/")[1]
		str_store_id = strings.Split(str_store_id, "?")[0]
		store_id, _ := strconv.Atoi(str_store_id)

		// получение полной информации о магазина по api
		http_req, _ := http.NewRequest("GET", fmt.Sprintf(`https://sbermarket.ru/api/stores/%d`,
			store_id), nil)
		http_req.Header.Set("User-Agent", userAgent)
		http_req.Header.Set("client-token", token)
		resp, _ := http_client.Do(http_req)
		json_store := Store{}
		err = json.NewDecoder(resp.Body).Decode(&json_store)
		if err != nil {
			log.Println("Failed to Unmarshal stores: ", err)
		}

		// переход на страницу магазина
		lastRequest.setNowTime()
		webdriver.Get(store_root)
		WaitForElement(webdriver, "//div[contains(@class, 'CategoryGrid_root')]", 10*time.Second)
		// получение кода страницы в виде строки
		raw, _ := webdriver.PageSource()
		// загрузка страницы в документ goquery
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
		if err != nil {
			log.Fatal(err)
		}
		// поиск категорий на странице и добавление их в общий список категорий
		doc.Find("div.CategoryGridItem_root__jXxOA").Each(func(i int, s *goquery.Selection) {
			url, _ := s.Find("a").Attr("href")
			title := s.Find("a").Text()
			categories.Add(Category{Shop: &json_store.Shop, Url: url, Name: title})
			fmt.Printf("Cat %s: %s\n", url, title)
		})
	}

	// для каждой категории из полученного списка происход переход по ссылке категории
	// и поиск товаров на открытой странице
	for _, category := range categories.All {

		// переход на страницу категории
		lastRequest.setNowTime()
		err := webdriver.Get(start_url + category.Url)
		if err != nil {
			log.Println("Failed to open url", category.Url, err)
			continue
		}
		// вывод всех товаров категории
		// здесь выполнено некорректно, т.к. существуют виртуальные категории, вроде
		// "спецпредложения" или "весенняя акция", у которых конфиграция страницы немного другая
		btnAllItems, err := webdriver.FindElement(
			selenium.ByXPATH,
			//LinkButton_default__4ZKEk LinkButton_primary___TA53
			`//a[contains(text(), "Все товары категории")]`)
		if err == nil {
			btnAllItems.Click()
		}
		WaitForElement(webdriver, "//div[contains(@class, 'ProductsGrid_root')]", 10*time.Second)
		// получение кода страницы и парсинг её в отдельной горутине
		raw, _ := webdriver.PageSource()
		wg_find_products.Add(1)
		go find_products(raw, category)
		// поиск следующих страниц и их парсинг (в данном случае берётся только одна следующая страница)
		for i := 0; i < 2; i++ {
			nextLink, err := webdriver.FindElement(selenium.ByXPATH, `//div[contains(@class, "pagination_next")]/a`)
			if err != nil {
				break
			}
			nextLink.Click()
			WaitForElement(webdriver, "//div[contains(@class, 'ProductsGrid_root')]", 10*time.Second)
			raw, _ := webdriver.PageSource()
			wg_find_products.Add(1)
			go find_products(raw, category)
		}
	}
	wg_find_products.Wait()
	err = saveProducts(products.All, fileName)
	if err != nil {
		log.Fatalf("Cannot create file %q: %s\n", fileName, err)
	}
}

func saveProducts(products []Product, fileName string) error {

	// создание пустого файла csv
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Comma = ';'
	defer writer.Flush()

	// Shop         string
	// ShopAddr     string
	// Category     *Category
	// Url          string
	// Name         string
	// PreviewImg   string
	// FullImg      string
	// CurrentPrice float64
	// FullPrice    float64
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
	writer.Write(headers)
	for _, product := range products {
		writer.Write([]string{
			product.Name,
			product.CurrentPrice,
			product.FullPrice,
			product.Shop,
			product.ShopAddr,
			product.Category.Name,
			product.PreviewImg,
			product.FullImg,
			product.Url,
		})
	}
	return nil
}

// поиск свободного порта
func pickUnusedPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

// enter text by KeyDown on every symbol
func printText(wd selenium.WebDriver, text string) error {
	for _, symb := range text {
		err := wd.KeyDown(string(symb))
		if err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

// очистка цены от лишних символов
func ClearPrice(price string, prefix string) (string, error) {
	// if _, err := fmt.Sscanf(s, " :%5d", &price); err == nil {
	// 	//
	// }
	price = strings.TrimPrefix(price, prefix)
	price = strings.ReplaceAll(price, "\u00a0", "")
	price = strings.ReplaceAll(price, "₽", "")
	// flPrice, err := strconv.ParseFloat(price, 32)
	// return flPrice, err
	return price, nil
}

// ожидание появления элемента на странице
func WaitForElement(webdriver selenium.WebDriver, strXPath string, waitTime time.Duration) error {
	err := webdriver.WaitWithTimeout(func(driver selenium.WebDriver) (bool, error) {
		element, _ := driver.FindElement(selenium.ByXPATH, strXPath)
		if element != nil {
			return element.IsDisplayed()
		}
		return false, nil
	}, waitTime)
	if err != nil {
		return err
	}
	return nil
}
