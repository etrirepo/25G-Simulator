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

// OctetStringClassID is the 16-bit ID for the OMCI
// Managed entity Octet string
const OctetStringClassID = ClassID(307) // 0x0133

var octetstringBME *ManagedEntityDefinition

// OctetString (Class ID: #307 / 0x0133)
//	The octet string is modelled on the large string ME. The large string is constrained to
//	printable characters because it uses null as a trailing delimiter. The octet string has a length
//	attribute and is therefore suitable for arbitrary sequences of bytes.
//
//	Instances of this ME are created and deleted by the OLT. To use this ME, the OLT instantiates
//	the octet string ME and then points to the created ME from other ME instances. Systems that
//	maintain the octet string should ensure that the octet string ME is not deleted while it is
//	still linked.
//
//	Relationships
//		An instance of this ME may be cited by any ME that requires an octet string that can exceed
//		25-bytes in length.
//
//	Attributes
//		Managed Entity Id
//			This attribute uniquely identifies each instance of this ME. The values 0 and 0xFFFF are
//			reserved. (R, setbycreate) (mandatory) (2-bytes)
//
//		Length
//			This attribute specifies the number of octets that comprise the sequence of octets. This
//			attribute defaults to 0 to indicate no octet string is defined. The maximum value of this
//			attribute is 375 (15 parts, 25-bytes each). (R,-W) (mandatory) (2-bytes)
//
//			In the following, 15 additional attributes are defined; they are identical. The octet string is
//			simply divided into as many parts as necessary, starting at part 1 and left justified.
//
//		Part 1
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 2
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 3
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 4
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 5
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 6
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 7
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 8
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 9
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 10
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 11
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 12
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 13
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 14
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
//		Part 15
//			Part 1, Part 2, Part 3, Part 4, Part 5, Part 6, Part 7, Part 8, Part 9,  Part 10, Part 11, Part
//			12, Part 13, Part 14, Part 15:  (R,-W) (part 1 mandatory, others optional) (25-bytes * 15
//			attributes)
//
type OctetString struct {
	ManagedEntityDefinition
	Attributes AttributeValueMap
}

// Attribute name constants

const OctetString_Length = "Length"
const OctetString_Part1 = "Part1"
const OctetString_Part2 = "Part2"
const OctetString_Part3 = "Part3"
const OctetString_Part4 = "Part4"
const OctetString_Part5 = "Part5"
const OctetString_Part6 = "Part6"
const OctetString_Part7 = "Part7"
const OctetString_Part8 = "Part8"
const OctetString_Part9 = "Part9"
const OctetString_Part10 = "Part10"
const OctetString_Part11 = "Part11"
const OctetString_Part12 = "Part12"
const OctetString_Part13 = "Part13"
const OctetString_Part14 = "Part14"
const OctetString_Part15 = "Part15"

func init() {
	octetstringBME = &ManagedEntityDefinition{
		Name:    "OctetString",
		ClassID: OctetStringClassID,
		MessageTypes: mapset.NewSetWith(
			Create,
			Delete,
			Get,
			Set,
		),
		AllowedAttributeMask: 0xffff,
		AttributeDefinitions: AttributeDefinitionMap{
			0:  Uint16Field(ManagedEntityID, PointerAttributeType, 0x0000, 0, mapset.NewSetWith(Read, SetByCreate), false, false, false, 0),
			1:  Uint16Field(OctetString_Length, UnsignedIntegerAttributeType, 0x8000, 0, mapset.NewSetWith(Read, Write), false, false, false, 1),
			2:  MultiByteField(OctetString_Part1, OctetsAttributeType, 0x4000, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, false, false, 2),
			3:  MultiByteField(OctetString_Part2, OctetsAttributeType, 0x2000, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 3),
			4:  MultiByteField(OctetString_Part3, OctetsAttributeType, 0x1000, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 4),
			5:  MultiByteField(OctetString_Part4, OctetsAttributeType, 0x0800, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 5),
			6:  MultiByteField(OctetString_Part5, OctetsAttributeType, 0x0400, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 6),
			7:  MultiByteField(OctetString_Part6, OctetsAttributeType, 0x0200, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 7),
			8:  MultiByteField(OctetString_Part7, OctetsAttributeType, 0x0100, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 8),
			9:  MultiByteField(OctetString_Part8, OctetsAttributeType, 0x0080, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 9),
			10: MultiByteField(OctetString_Part9, OctetsAttributeType, 0x0040, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 10),
			11: MultiByteField(OctetString_Part10, OctetsAttributeType, 0x0020, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 11),
			12: MultiByteField(OctetString_Part11, OctetsAttributeType, 0x0010, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 12),
			13: MultiByteField(OctetString_Part12, OctetsAttributeType, 0x0008, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 13),
			14: MultiByteField(OctetString_Part13, OctetsAttributeType, 0x0004, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 14),
			15: MultiByteField(OctetString_Part14, OctetsAttributeType, 0x0002, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 15),
			16: MultiByteField(OctetString_Part15, OctetsAttributeType, 0x0001, 25, toOctets("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="), mapset.NewSetWith(Read, Write), false, true, false, 16),
		},
		Access:  CreatedByOlt,
		Support: UnknownSupport,
	}
}

// NewOctetString (class ID 307) creates the basic
// Managed Entity definition that is used to validate an ME of this type that
// is received from or transmitted to the OMCC.
func NewOctetString(params ...ParamData) (*ManagedEntity, OmciErrors) {
	return NewManagedEntity(*octetstringBME, params...)
}
