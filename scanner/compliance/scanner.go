package compliance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/deepfence/kubernetes-scanner/util"
	"github.com/sirupsen/logrus"
)

type ComplianceScanner struct {
	config   util.Config
	scanID   string
	scanType string
}

func NewComplianceScanner(config util.Config, scanID string, scanType string) (*ComplianceScanner, error) {
	if scanID != util.NsaCisaCheckType {
		return nil, errors.New(fmt.Sprintf("invalid scan_type %s", scanType))
	}
	if scanID == "" {
		return nil, errors.New("scan_id is empty")
	}
	return &ComplianceScanner{
		config:   config,
		scanID:   scanID,
		scanType: scanType,
	}, nil
}

func (c *ComplianceScanner) RunComplianceScan() error {
	err := c.PublishScanStatus("", "INPROGRESS", nil)
	if err != nil {
		return err
	}
	tempFileName := fmt.Sprintf("/tmp/%s.json", util.RandomString(12))
	//defer os.Remove(tempFileName)
	spKubePath := "/opt/steampipe/steampipe-mod-kubernetes-compliance"
	cmd := fmt.Sprintf("cd %s && steampipe check --progress=false --output=none --export=%s benchmark.nsa_cisa_v1", spKubePath, tempFileName)
	stdOut, stdErr := exec.Command("bash", "-c", cmd).CombinedOutput()
	var complianceResults util.ComplianceGroup
	if _, err := os.Stat(tempFileName); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %v", stdOut, stdErr)
	}
	tempFile, err := os.Open(tempFileName)
	if err != nil {
		return err
	}
	results, err := io.ReadAll(tempFile)
	if err != nil {
		return err
	}
	err = json.Unmarshal(results, &complianceResults)
	if err != nil {
		return err
	}
	complianceDocs, complianceSummary, err := c.ParseComplianceResults(complianceResults)
	if err != nil {
		return err
	}
	err = c.IngestComplianceResults(complianceDocs)
	if err != nil {
		logrus.Error(err)
	}
	extras := map[string]interface{}{
		"node_name":    c.config.NodeName,
		"node_id":      c.config.NodeId,
		"result":       complianceSummary,
		"total_checks": complianceSummary.Alarm + complianceSummary.Ok + complianceSummary.Info + complianceSummary.Skip + complianceSummary.Error,
	}
	err = c.PublishScanStatus("", "COMPLETED", extras)
	if err != nil {
		logrus.Error(err)
	}
	return nil
}

func (c *ComplianceScanner) PublishScanStatus(scanMsg string, status string, extras map[string]interface{}) error {
	scanMsg = strings.Replace(scanMsg, "\n", " ", -1)
	scanLog := map[string]interface{}{
		"scan_id":                 c.scanID,
		"time_stamp":              util.GetIntTimestamp(),
		"@timestamp":              util.GetDatetimeNow(),
		"scan_message":            scanMsg,
		"scan_status":             status,
		"type":                    util.ComplianceScanLogs,
		"node_name":               c.config.NodeName,
		"node_id":                 c.config.NodeId,
		"kubernetes_cluster_name": c.config.NodeName,
		"kubernetes_cluster_id":   c.config.NodeId,
		"compliance_check_type":   c.scanType,
	}
	for k, v := range extras {
		scanLog[k] = v
	}
	scanLogJson := util.ToKafkaRestFormat([]map[string]interface{}{scanLog})
	// TODO: Save to file
	if len(scanLogJson) > 0 {

	}
	return nil
}

func (c *ComplianceScanner) IngestComplianceResults(complianceDocs []util.ComplianceDoc) error {
	logrus.Debugf("Number of docs to ingest: %d", len(complianceDocs))
	data := make([]map[string]interface{}, len(complianceDocs))
	for index, complianceDoc := range complianceDocs {
		mapData, err := util.StructToMap(complianceDoc)
		if err == nil {
			data[index] = mapData
		} else {
			logrus.Error(err)
		}
	}
	// TODO: Save to file

	//ingestScanStatusAPI := fmt.Sprintf("https://" + config.ManagementConsoleUrl + "/ingest/topics/" + util.ComplianceScanIndexName)
	//return util.PublishDocument(ingestScanStatusAPI, util.ToKafkaRestFormat(data), config)
	return nil
}
