package geolocation

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	geoLiteCityTarGZURLEnv     = "ANONBIRD_GEOLITE_CITY_TAR_URL"
	geoLiteCityZipURLEnv       = "ANONBIRD_GEOLITE_CITY_CSV_ZIP_URL"
	geoLiteCitySha256TarURLEnv = "ANONBIRD_GEOLITE_CITY_TAR_SHA256_URL"
	geoLiteCitySha256ZipURLEnv = "ANONBIRD_GEOLITE_CITY_CSV_ZIP_SHA256_URL"
	geoLiteCityMMDB            = "GeoLite2-City.mmdb"
	geoLiteCityCSV             = "GeoLite2-City-Locations-en.csv"
)

// loadGeolocationDatabases loads the MaxMind databases.
func loadGeolocationDatabases(ctx context.Context, dataDir string, mmdbFile string, geonamesdbFile string) error {
	for _, file := range []string{mmdbFile, geonamesdbFile} {
		exists, _ := fileExists(path.Join(dataDir, file))
		if exists {
			continue
		}

		log.WithContext(ctx).Infof("Geolocation database file %s not found, file will be downloaded", file)

		switch file {
		case mmdbFile:
			fileURL, checksumURL, err := geoliteDownloadURLs(geoLiteCityTarGZURLEnv, geoLiteCitySha256TarURLEnv)
			if err != nil {
				return err
			}
			extractFunc := func(src string, dst string) error {
				if err := decompressTarGzFile(src, dst); err != nil {
					return err
				}
				return copyFile(path.Join(dst, geoLiteCityMMDB), path.Join(dataDir, mmdbFile))
			}
			if err := loadDatabase(
				checksumURL,
				fileURL,
				extractFunc,
			); err != nil {
				return err
			}

		case geonamesdbFile:
			fileURL, checksumURL, err := geoliteDownloadURLs(geoLiteCityZipURLEnv, geoLiteCitySha256ZipURLEnv)
			if err != nil {
				return err
			}
			extractFunc := func(src string, dst string) error {
				if err := decompressZipFile(src, dst); err != nil {
					return err
				}
				extractedCsvFile := path.Join(dst, geoLiteCityCSV)
				return importCsvToSqlite(dataDir, extractedCsvFile, geonamesdbFile)
			}

			if err := loadDatabase(
				checksumURL,
				fileURL,
				extractFunc,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func geoliteDownloadURLs(fileEnv string, checksumEnv string) (string, string, error) {
	fileURL := strings.TrimSpace(os.Getenv(fileEnv))
	checksumURL := strings.TrimSpace(os.Getenv(checksumEnv))
	if fileURL == "" || checksumURL == "" {
		return "", "", fmt.Errorf("geolocation database is not present and download URLs are not configured; set %s and %s or pre-seed the database files", fileEnv, checksumEnv)
	}
	return fileURL, checksumURL, nil
}

// loadDatabase downloads a file from the specified URL and verifies its checksum.
// It then calls the extract function to perform additional processing on the extracted files.
func loadDatabase(checksumURL string, fileURL string, extractFunc func(src string, dst string) error) error {
	temp, err := os.MkdirTemp(os.TempDir(), "geolite")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)

	checksumFilename, err := getFilenameFromURL(checksumURL)
	if err != nil {
		return err
	}
	checksumFile := path.Join(temp, checksumFilename)

	err = downloadFile(checksumURL, checksumFile)
	if err != nil {
		return err
	}

	sha256sum, err := loadChecksumFromFile(checksumFile)
	if err != nil {
		return err
	}

	dbFilename, err := getFilenameFromURL(fileURL)
	if err != nil {
		return err
	}
	dbFile := path.Join(temp, dbFilename)

	err = downloadFile(fileURL, dbFile)
	if err != nil {
		return err
	}

	if err := verifyChecksum(dbFile, sha256sum); err != nil {
		return err
	}

	return extractFunc(dbFile, temp)
}

// importCsvToSqlite imports a CSV file into a SQLite database.
func importCsvToSqlite(dataDir string, csvFile string, geonamesdbFile string) error {
	geonames, err := loadGeonamesCsv(csvFile)
	if err != nil {
		return err
	}

	db, err := gorm.Open(sqlite.Open(path.Join(dataDir, geonamesdbFile)), &gorm.Config{
		Logger:          logger.Default.LogMode(logger.Silent),
		CreateBatchSize: 1000,
	})
	if err != nil {
		return err
	}
	defer func() {
		sql, err := db.DB()
		if err != nil {
			return
		}
		sql.Close()
	}()

	if err := db.AutoMigrate(&GeoNames{}); err != nil {
		return err
	}

	return db.Create(geonames).Error
}

func loadGeonamesCsv(filepath string) ([]GeoNames, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var geoNames []GeoNames
	for index, record := range records {
		if index == 0 {
			continue
		}
		geoNameID, err := strconv.Atoi(record[0])
		if err != nil {
			return nil, err
		}

		geoName := GeoNames{
			GeoNameID:           geoNameID,
			LocaleCode:          record[1],
			ContinentCode:       record[2],
			ContinentName:       record[3],
			CountryIsoCode:      record[4],
			CountryName:         record[5],
			Subdivision1IsoCode: record[6],
			Subdivision1Name:    record[7],
			Subdivision2IsoCode: record[8],
			Subdivision2Name:    record[9],
			CityName:            record[10],
			MetroCode:           record[11],
			TimeZone:            record[12],
			IsInEuropeanUnion:   record[13],
		}
		geoNames = append(geoNames, geoName)
	}

	return geoNames, nil
}

// copyFile performs a file copy operation from the source file to the destination.
func copyFile(src string, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	return nil
}
