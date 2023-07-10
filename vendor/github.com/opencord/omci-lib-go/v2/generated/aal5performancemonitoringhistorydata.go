/*
 * Copyright (c) 2018 - present.  Boling Consulting Solutions (bcsw.net)
 * Copyright 2020-present Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
/*
 * NOTE: This file was generated, manual edits will be overwritten!
 *
 * Generated by 'goCodeGenerator.py':
 *              https://github.com/cboling/OMCI-parser/README.md
 */

package generated

import "github.com/deckarep/golang-set"

// Aal5PerformanceMonitoringHistoryDataClassID is the 16-bit ID for the OMCI
// Managed entity AAL5 performance monitoring history data
const Aal5PerformanceMonitoringHistoryDataClassID = ClassID(18) // 0x0012

var aal5performancemonitoringhistorydataBME *ManagedEntityDefinition

// Aal5PerformanceMonitoringHistoryData (Class ID: #18 / 0x0012)
//	This ME collects PM data as a result of performing segmentation and reassembly (SAR) and
//	convergence sublayer (CS) level protocol monitoring. Instances of this ME are created and
//	deleted by the OLT.
//
//	For a complete discussion of generic PM architecture, refer to clause I.4.
//
//	Relationships
//		An instance of this ME is associated with an instance of an IW VCC TP that represents AAL5
//		functions.
//
//	Attributes
//		Managed Entity Id
//			This attribute uniquely identifies each instance of this ME. Through an identical ID, this ME is
//			implicitly linked to an instance of the IW VCC TP. (R, setbycreate) (mandatory) (2-bytes)
//
//		Interval End Time
//			This attribute identifies the most recently finished 15-min interval. (R) (mandatory) (1-byte)
//
//		Threshold Data 1_2 Id
//			Threshold data 1/2 ID: This attribute points to an instance of the threshold data 1 ME that
//			contains PM threshold values. Since no threshold value attribute number exceeds 7, a threshold
//			data 2 ME is optional. (R,-W, setbycreate) (mandatory) (2-bytes)
//
//		Sum Of Invalid Cs Field Errors
//			This attribute counts the sum of invalid CS field errors. For AAL type 5, this attribute is a
//			single count of the number of CS PDUs discarded due to one of the following error conditions:
//			invalid common part indicator (CPI), oversized received SDU, or length violation. (R)
//			(mandatory) (4-bytes)
//
//		Crc Violations
//			This attribute counts CRC violations detected on incoming SAR PDUs. (R) (mandatory) (4-bytes)
//
//		Reassembly Timer Expirations
//			This attribute counts reassembly timer expirations. (R) (mandatory if reassembly timer is
//			implemented) (4-bytes)
//
//		Buffer Overflows
//			This attribute counts the number of times where there was not enough buffer space for a
//			reassembled packet. (R) (mandatory) (4-bytes)
//
//		Encap Protocol Errors
//			This attribute counts the number of times that [IETF RFC 2684] encapsulation protocol detected a
//			bad header. (R) (mandatory) (4-bytes)
//
type Aal5PerformanceMonitoringHistoryData struct {
	ManagedEntityDefinition
	Attributes AttributeValueMap
}

// Attribute name constants

const Aal5PerformanceMonitoringHistoryData_IntervalEndTime = "IntervalEndTime"
const Aal5PerformanceMonitoringHistoryData_ThresholdData12Id = "ThresholdData12Id"
const Aal5PerformanceMonitoringHistoryData_SumOfInvalidCsFieldErrors = "SumOfInvalidCsFieldErrors"
const Aal5PerformanceMonitoringHistoryData_CrcViolations = "CrcViolations"
const Aal5PerformanceMonitoringHistoryData_ReassemblyTimerExpirations = "ReassemblyTimerExpirations"
const Aal5PerformanceMonitoringHistoryData_BufferOverflows = "BufferOverflows"
const Aal5PerformanceMonitoringHistoryData_EncapProtocolErrors = "EncapProtocolErrors"

func init() {
	aal5performancemonitoringhistorydataBME = &ManagedEntityDefinition{
		Name:    "Aal5PerformanceMonitoringHistoryData",
		ClassID: Aal5PerformanceMonitoringHistoryDataClassID,
		MessageTypes: mapset.NewSetWith(
			Create,
			Delete,
			Get,
			Set,
			GetCurrentData,
		),
		AllowedAttributeMask: 0xfe00,
		AttributeDefinitions: AttributeDefinitionMap{
			0: Uint16Field(ManagedEntityID, PointerAttributeType, 0x0000, 0, mapset.NewSetWith(Read, SetByCreate), false, false, false, 0),
			1: ByteField(Aal5PerformanceMonitoringHistoryData_IntervalEndTime, UnsignedIntegerAttributeType, 0x8000, 0, mapset.NewSetWith(Read), false, false, false, 1),
			2: Uint16Field(Aal5PerformanceMonitoringHistoryData_ThresholdData12Id, UnsignedIntegerAttributeType, 0x4000, 0, mapset.NewSetWith(Read, SetByCreate, Write), false, false, false, 2),
			3: Uint32Field(Aal5PerformanceMonitoringHistoryData_SumOfInvalidCsFieldErrors, CounterAttributeType, 0x2000, 0, mapset.NewSetWith(Read), false, false, false, 3),
			4: Uint32Field(Aal5PerformanceMonitoringHistoryData_CrcViolations, CounterAttributeType, 0x1000, 0, mapset.NewSetWith(Read), false, false, false, 4),
			5: Uint32Field(Aal5PerformanceMonitoringHistoryData_ReassemblyTimerExpirations, CounterAttributeType, 0x0800, 0, mapset.NewSetWith(Read), false, false, false, 5),
			6: Uint32Field(Aal5PerformanceMonitoringHistoryData_BufferOverflows, CounterAttributeType, 0x0400, 0, mapset.NewSetWith(Read), false, false, false, 6),
			7: Uint32Field(Aal5PerformanceMonitoringHistoryData_EncapProtocolErrors, CounterAttributeType, 0x0200, 0, mapset.NewSetWith(Read), false, false, false, 7),
		},
		Access:  CreatedByOlt,
		Support: UnknownSupport,
		Alarms: AlarmMap{
			0: "Invalid fields",
			1: "CRC violation",
			2: "Reassembly timer expirations",
			3: "Buffer overflows",
			4: "Encap protocol errors",
		},
	}
}

// NewAal5PerformanceMonitoringHistoryData (class ID 18) creates the basic
// Managed Entity definition that is used to validate an ME of this type that
// is received from or transmitted to the OMCC.
func NewAal5PerformanceMonitoringHistoryData(params ...ParamData) (*ManagedEntity, OmciErrors) {
	return NewManagedEntity(*aal5performancemonitoringhistorydataBME, params...)
}
