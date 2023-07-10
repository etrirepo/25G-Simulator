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

// MulticastSubscriberMonitorClassID is the 16-bit ID for the OMCI
// Managed entity Multicast subscriber monitor
const MulticastSubscriberMonitorClassID = ClassID(311) // 0x0137

var multicastsubscribermonitorBME *ManagedEntityDefinition

// MulticastSubscriberMonitor (Class ID: #311 / 0x0137)
//	This ME provides the current status of each port with respect to its multicast subscriptions. It
//	may be useful for status monitoring or debugging purposes. The status table includes all dynamic
//	groups currently subscribed by the port.
//
//	Relationships
//		Instances of this ME are created and deleted at the request of the OLT. One instance may exist
//		for each IEEE-802.1 UNI configured to support multicast subscription.
//
//	Attributes
//		Managed Entity Id
//			This attribute uniquely identifies each instance of this ME. Through an identical ID, this ME is
//			implicitly linked to an instance of the MAC bridge port configuration data or IEEE-802.1p mapper
//			ME. (R,-setbycreate) (mandatory) (2-bytes)
//
//		Me Type
//			This attribute indicates the type of the ME implicitly linked by the ME ID attribute.
//
//			0	MAC bridge port config data
//
//			1	IEEE-802.1p mapper service profile
//
//			(R,-W, setbycreate) (mandatory) (1-byte)
//
//		Current Multicast Bandwidth
//			This attribute is the ONU's (BE) estimate of the actual bandwidth currently being delivered to
//			this particular MAC bridge port over all dynamic multicast groups. (R) (optional) (4-bytes)
//
//		Join Messages Counter
//			This attribute counts the number of times the corresponding subscriber sent a join message that
//			was accepted. When full, the counter rolls over to 0. (R) (optional) (4-bytes)
//
//		Bandwidth Exceeded Counter
//			This attribute counts the number of join messages that did exceed, or would have exceeded, the
//			max multicast bandwidth, whether accepted or denied. When full, the counter rolls over to 0. (R)
//			(optional) (4-bytes)
//
//		Ipv4 Active Group List Table
//			This attribute lists the groups from one of the related dynamic access control list tables or
//			the allowed preview groups table that are currently being actively forwarded, along with the
//			actual bandwidth of each. If a join has been recognized from more than one IPv4 source address
//			for a given group on this UNI, there will be one table entry for each. Each table entry has the
//			following form.
//
//			-	VLAN ID, 0 if not used (2-bytes)
//
//			-	Source IP address, 0.0.0.0 if not used (4-bytes)
//
//			-	Multicast destination IP address (4-bytes)
//
//			-	Best efforts actual bandwidth estimate, bytes per second (4-bytes)
//
//			-	Client (set-top box) IP address, i.e., the IP address of the device currently joined (4-bytes)
//
//			-	Time since the most recent join of this client to the IP channel, in seconds (4-bytes)
//
//			-	Reserved (2-bytes)
//
//			(R) (mandatory) (24N bytes)
//
//		Ipv6 Active Group List Table
//			-	Time since the most recent join of this client to the IP channel, in seconds (4-bytes)
//
//			(R) (optional) (58N bytes)
//
//			This attribute lists the groups from one of the related dynamic access control list tables or
//			the allowed preview groups table that are currently being actively forwarded, along with the
//			actual bandwidth of each. If a join has been recognized from more than one IPv6 source address
//			for a given group on this UNI, there will be one table entry for each. In mixed IPv4-IPv6
//			scenarios, it is possible that some fields might be IPv4, in which case their 12 most
//			significant bytes of the given field are set to zero. Each table entry has the form:
//
//			-	VLAN ID, 0 if not used (2-bytes)
//
//			-	Source IP address, 0 if not used (16-bytes)
//
//			-	Multicast destination IP address (16-bytes)
//
//			-	Best efforts actual bandwidth estimate, bytes per second (4-bytes)
//
//			-	Client (set-top box) IP address, i.e., the IP address of the device currently joined
//			(16-bytes)
//
type MulticastSubscriberMonitor struct {
	ManagedEntityDefinition
	Attributes AttributeValueMap
}

// Attribute name constants

const MulticastSubscriberMonitor_MeType = "MeType"
const MulticastSubscriberMonitor_CurrentMulticastBandwidth = "CurrentMulticastBandwidth"
const MulticastSubscriberMonitor_JoinMessagesCounter = "JoinMessagesCounter"
const MulticastSubscriberMonitor_BandwidthExceededCounter = "BandwidthExceededCounter"
const MulticastSubscriberMonitor_Ipv4ActiveGroupListTable = "Ipv4ActiveGroupListTable"
const MulticastSubscriberMonitor_Ipv6ActiveGroupListTable = "Ipv6ActiveGroupListTable"

func init() {
	multicastsubscribermonitorBME = &ManagedEntityDefinition{
		Name:    "MulticastSubscriberMonitor",
		ClassID: MulticastSubscriberMonitorClassID,
		MessageTypes: mapset.NewSetWith(
			Create,
			Delete,
			Get,
			GetNext,
			Set,
		),
		AllowedAttributeMask: 0xfc00,
		AttributeDefinitions: AttributeDefinitionMap{
			0: Uint16Field(ManagedEntityID, PointerAttributeType, 0x0000, 0, mapset.NewSetWith(Read, SetByCreate), false, false, false, 0),
			1: ByteField(MulticastSubscriberMonitor_MeType, EnumerationAttributeType, 0x8000, 0, mapset.NewSetWith(Read, SetByCreate, Write), false, false, false, 1),
			2: Uint32Field(MulticastSubscriberMonitor_CurrentMulticastBandwidth, UnsignedIntegerAttributeType, 0x4000, 0, mapset.NewSetWith(Read), false, true, false, 2),
			3: Uint32Field(MulticastSubscriberMonitor_JoinMessagesCounter, UnsignedIntegerAttributeType, 0x2000, 0, mapset.NewSetWith(Read), false, true, false, 3),
			4: Uint32Field(MulticastSubscriberMonitor_BandwidthExceededCounter, UnsignedIntegerAttributeType, 0x1000, 0, mapset.NewSetWith(Read), false, true, false, 4),
			5: TableField(MulticastSubscriberMonitor_Ipv4ActiveGroupListTable, TableAttributeType, 0x0800, TableInfo{nil, 24}, mapset.NewSetWith(Read), false, false, false, 5),
			6: TableField(MulticastSubscriberMonitor_Ipv6ActiveGroupListTable, TableAttributeType, 0x0400, TableInfo{nil, 58}, mapset.NewSetWith(Read), false, true, false, 6),
		},
		Access:  CreatedByOlt,
		Support: UnknownSupport,
	}
}

// NewMulticastSubscriberMonitor (class ID 311) creates the basic
// Managed Entity definition that is used to validate an ME of this type that
// is received from or transmitted to the OMCC.
func NewMulticastSubscriberMonitor(params ...ParamData) (*ManagedEntity, OmciErrors) {
	return NewManagedEntity(*multicastsubscribermonitorBME, params...)
}
