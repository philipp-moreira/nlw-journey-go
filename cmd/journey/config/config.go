package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const PREFIX_ENVIRONMENT_VARIABLES = "JOURNEY_"
const SPLIT_OPERATOR_ENVIRONMENT_VARIABLES = "="
const DEFAULT_PATH_TO_ENVIRONMENT_VARIABLES_FILE = "../../../.env"

func GetSpecificEnvironmentVariable(key string) (string, error) {

	var stringEmpty = ""

	variables, err := GetEnvironmentVariables()
	if err != nil {
		return stringEmpty, err
	}

	variable := variables[key]

	return variable, nil
}

func GetEnvironmentVariables() (map[string]string, error) {

	var variables map[string]string

	variables, err := getEnvironmentVariablesFromOS()
	if err != nil {
		variables, err = getEnvironmentVariablesFromEnvFile()
		if err != nil {
			return nil, errors.New("environment variables don't found in os and .env file")
		}
	}

	return variables, nil
}

func getSpecificEnvironmentVariableFromEnvFile(key string) (string, error) {

	responseEmpty := ""

	variables, err := getEnvironmentVariablesFromEnvFile()
	if err != nil {
		return responseEmpty, err
	}

	value := variables[key]

	return value, err
}

func getSpecificEnvironmentVariableFromOs(key string) (string, error) {

	responseEmpty := ""

	variables, err := getEnvironmentVariablesFromOS()
	if err != nil {
		return responseEmpty, err
	}

	value := variables[key]

	return value, nil
}

func getEnvironmentVariablesFromEnvFile() (map[string]string, error) {

	err := godotenv.Load(DEFAULT_PATH_TO_ENVIRONMENT_VARIABLES_FILE)
	if err != nil {
		return nil, errors.New(err.Error())
	}

	dictionaryVariables := make(map[string]string)
	variables := os.Environ()
	variablesFiltered := filterApplicationEnvironmentVariables(variables)

	if len(variablesFiltered) == 0 {
		return nil, errors.New(fmt.Sprintf("environment variables don't found in .env file '%s'", DEFAULT_PATH_TO_ENVIRONMENT_VARIABLES_FILE))
	}

	for _, row := range variablesFiltered {

		fieldsValue := strings.Split(row, SPLIT_OPERATOR_ENVIRONMENT_VARIABLES)

		key := strings.Trim(fieldsValue[0], " ")
		value := strings.Trim(fieldsValue[1], " ")

		dictionaryVariables[key] = value
	}

	return dictionaryVariables, err
}

func getEnvironmentVariablesFromOS() (map[string]string, error) {

	dictionaryVariables := make(map[string]string)
	variables := os.Environ()
	variablesFiltered := filterApplicationEnvironmentVariables(variables)

	if len(variablesFiltered) == 0 {
		return nil, errors.New("environment variables don't found in os")
	}

	for _, row := range variablesFiltered {

		fieldsValue := strings.Split(row, SPLIT_OPERATOR_ENVIRONMENT_VARIABLES)

		key := strings.Trim(fieldsValue[0], " ")
		value := strings.Trim(fieldsValue[1], " ")

		dictionaryVariables[key] = value
	}

	return dictionaryVariables, nil
}

func filterApplicationEnvironmentVariables(variables []string) []string {
	variablesFiltered := make([]string, 0)

	for _, row := range variables {
		if strings.HasPrefix(row, PREFIX_ENVIRONMENT_VARIABLES) {
			variablesFiltered = append(variablesFiltered, row)
		}
	}

	return variablesFiltered
}
