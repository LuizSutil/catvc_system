package main

import (
	"os"
	"io/ioutil"
	"strings"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"unicode"
	"math"
	"sync"
	"encoding/json"
	"time"
	"sort"
	"github.com/robinson/gos7"
	"github.com/prometheus/client_golang/prometheus"
  "github.com/prometheus/client_golang/prometheus/promhttp"

)

const (
	//PLC stuff
	tcpDevice = "10.0.17.51"
	rack      = 0
	slot      = 0
)

type PlcStructure struct {
  ValueType 	string 	`json:"value_type"`
  DbAddress 	string	`json:"db_address"`
  ReadInterval 	int	`json:"read_interval"`
  dbStart int
}

type DbReadMap struct{
	dbNumber int
	minVal,maxVal int
	addresses []PlcStructure
	readInterval time.Duration
	lastRead time.Time
}


var (
	mu sync.Mutex
	connection = false

  tagData = prometheus.NewGaugeVec(prometheus.GaugeOpts{
    Name: "PLC_tag_data",
    Help: "Current value of a tag",
  },
  []string{"tag"},)
	 tagError = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	  Name: "PLC_tag_Error",
	  Help: "Tag Read Error",
	},
	[]string{"tagError"},)
)

func init(){
    prometheus.MustRegister(tagData)
    prometheus.MustRegister(tagError)
}


func main(){

	handler := gos7.NewTCPClientHandler(tcpDevice, rack, slot)
	// handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)
	// handler.Timeout = 200 * time.Second
	// handler.IdleTimeout = 200 * time.Second
	handler.Connect()
	defer handler.Close()
	plcClient := gos7.NewClient(handler)


	go RecoveryProcedure(&plcClient)

	mydbs,	mydbsPriority :=  dbMapper()
	
	// make this its own go routine

	go ReadLoop(mydbs,mydbsPriority ,&plcClient)

 	http.Handle("/metrics", promhttp.Handler())
  log.Fatal(http.ListenAndServe(":8080", nil))


}

func RecoveryProcedure(plcClient *gos7.Client){
	for{
		time.Sleep(1 * time.Second)
		for connection == false{
			handler := gos7.NewTCPClientHandler(tcpDevice, rack, slot)
			err := handler.Connect()
			if err != nil{
				fmt.Println(err)
			} else {
				fmt.Println("Connection Reestablished")
				*plcClient = gos7.NewClient(handler)
				connection = true
				tagError.With(prometheus.Labels{"tagError":tcpDevice}).Set(1)

			}
			
		}
	}
}

func ConStatusChange(status bool){
	mu.Lock()
	connection = status
	mu.Unlock()

}

func ReadLoop(mydbs []DbReadMap,mydbsPriority []DbReadMap,plcClient *gos7.Client ){

	for{

 		startTime := time.Now()


 		// read 100ms db

 		for _,priorityDb := range mydbsPriority{
 			ReadDB(priorityDb,plcClient)
 		}
 	

 		// time limite for reading other DBs, can become dynanmic in future
 		timeLimit := time.Duration(68 * time.Millisecond)

 		for time.Since(startTime) < timeLimit{
 			currRead := mydbs[0]

			// Ver quanto tempo desde a minha ultima leitura
			// Ver se o tempo desde de a minha ultima leitura da variavel
			// é maior do que o intervalo de leitura dela
			if time.Since(currRead.lastRead) >= currRead.readInterval{

				// Ler variavel
				ReadDB(currRead, plcClient)

				// definir ultima leitura da variavel como agora
				mydbs[0].lastRead = time.Now()

				mydbs = queueSort(mydbs)
 			}
 		}

 		// waits the rest of the 100ms to not mess up 100ms read time
	 		for time.Since(startTime) < time.Duration(100* time.Millisecond){
				// time.Sleep(10 * time.Millisecond)
				// fmt.Println("downtime")
			}
 		}	
}

// also takes in gos7 handler
func ReadDB(dbBlock DbReadMap, plcClient *gos7.Client){
	if connection == false{
		return
	}	

	buffer := make([]byte, dbBlock.maxVal + 4)
	client := *plcClient
	readErr := client.AGReadDB(dbBlock.dbNumber,dbBlock.minVal,dbBlock.maxVal + 4, buffer)
	if readErr != nil{
		ConStatusChange(false)
		fmt.Println(readErr)
		tagError.With(prometheus.Labels{"tagError":tcpDevice}).Set(0)
		return
	}
	// divide up dbBlock.addresses

	var dividedBlock [][]PlcStructure

	arrLen := len(dbBlock.addresses)

	chunkSize := (arrLen + 8 - 1) / 8

	for i := 0; i < arrLen; i += chunkSize {
		end := i + chunkSize

		if end > arrLen {
			end = arrLen
		}

		dividedBlock = append(dividedBlock, dbBlock.addresses[i:end])
	}

	for _,v := range dividedBlock{
		// is this gonna run?
		go dumpBufferVals(buffer,v)
	}
}

func dumpBufferVals(buffer []byte, addresses []PlcStructure){
	var s7 gos7.Helper
	for _, unit := range addresses{
		switch unit.ValueType {
		    case "Real":
					result := s7.GetRealAt(buffer, unit.dbStart - 2) 
					value := result

					fmt.Println(result)
					// write tag to prometheus object
					tagData.With(prometheus.Labels{"tag":unit.DbAddress}).Set(float64(value))

		    case "Boolean":
			    var result bool
					s7.GetValueAt(buffer, unit.dbStart, &result) 
					value := strconv.FormatBool(result)
					// write tag to prometheus object
					var floatBool float64
					if value == "true"{
						floatBool = 1
					} else {
						floatBool = 0
					}
					tagData.With(prometheus.Labels{"tag":unit.DbAddress}).Set(floatBool)
		    default:
		        fmt.Println("type not found")
	    }
	}
}

func queueSort(dbList []DbReadMap)[]DbReadMap{

	t := dbList
	
	sort.Slice(dbList, func(i, j int) bool {

		a := dbList[i].readInterval - time.Since(dbList[i].lastRead)
		b := dbList[j].readInterval - time.Since(dbList[j].lastRead) 

    	return a < b 
	})

	return t
}

func dbMapper() ([]DbReadMap, []DbReadMap){


	jsonFile, err := os.Open("./tagList.json")
	// if we os.Open returns an error then handle it
	if err != nil {
	    fmt.Println(err)
	}

	byteValue, _ := ioutil.ReadAll(jsonFile)
	// myJson := `[{"value_type": "real","db_address": "DB1.DBD2", "read_interval":1000},
	// 			{"value_type": "real","db_address": "DB1.DBD6", "read_interval":1000},
	// 			{"value_type": "real","db_address": "DB1.DBD10", "read_interval":1000},
	// 			{"value_type": "real","db_address": "DB17.DBD14", "read_interval":100}]`

	var plcStructures []PlcStructure
	json.Unmarshal([]byte(byteValue), &plcStructures)	

	// Separate json into each dbBlock
	separateDbs := make(map[string][]PlcStructure)
	for _,address := range plcStructures{

		parsedAddress := strings.SplitN(address.DbAddress, ".",2)
		dbNumber := parsedAddress[0]

		addressStartString := ""
		for _,s := range parsedAddress[1] {
				if !unicode.IsLetter(s){
					addressStartString = addressStartString + string(s)
					// fmt.Println(string(s))
				}
			}
		value,_ := strconv.ParseFloat(addressStartString,64)
		
		address.dbStart = int(value)
		// dbValue := parsedAddress[1]

		separateDbs[dbNumber] = append(separateDbs[dbNumber],address)
	}



	var maps []DbReadMap
	var mapsPriority []DbReadMap

	for db, addressMaps := range separateDbs{

		var dbReadMap DbReadMap

			largestVal := math.Inf(-1)
    	smallestVal := math.Inf(1)
    	dbReadMap.readInterval = time.Duration(100000000000)

    	for _, addressMap := range addressMaps{

    		floatStart := float64(addressMap.dbStart)

    		if floatStart < smallestVal{
				smallestVal = floatStart
			}
			if floatStart > largestVal {
				largestVal = floatStart
			}

			readIntervalDuration := time.Duration(addressMap.ReadInterval) * time.Millisecond
			if readIntervalDuration < dbReadMap.readInterval{
				dbReadMap.readInterval = time.Duration(addressMap.ReadInterval) * time.Millisecond
			}
    	}

    	var dbToIntString string
    	for _,s := range db {
			if !unicode.IsLetter(s){
				dbToIntString = dbToIntString + string(s)
				// fmt.Println(string(s))
			}
		}
		dbInt,_ := strconv.Atoi(dbToIntString)
    
    	dbReadMap.dbNumber = dbInt
    	dbReadMap.minVal = int(smallestVal)
    	dbReadMap.maxVal = int(largestVal)
    	dbReadMap.addresses = addressMaps  
    	dbReadMap.lastRead = time.Time{}  

    	if dbReadMap.readInterval == time.Duration(100 * time.Millisecond){
    		mapsPriority = append(mapsPriority,dbReadMap)
    	} else{
    		maps = append(maps,dbReadMap)	
    	}

	}

	return maps,mapsPriority
}