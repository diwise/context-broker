package entities

import (
	"encoding/json"
	"testing"

	"github.com/matryer/is"
)

func TestJSONMarshalling(t *testing.T) {
	is := is.New(t)
	e, err := NewFromJSON([]byte(entityJSON))

	is.NoErr(err)
	is.Equal(e.ID(), "urn:ngsi-ld:WeatherObserved:observationid")
	is.Equal(e.Type(), "WeatherObserved")

	b, err := json.Marshal(e)

	is.NoErr(err)
	is.Equal(string(b), "{\"@context\":[\"https://schema.lab.fiware.org/ld/context\",\"https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld\"],\"id\":\"urn:ngsi-ld:WeatherObserved:observationid\",\"location\":{\"type\":\"GeoProperty\",\"value\":{\"type\":\"Point\",\"coordinates\":[-8.768460000000001,42.60214472222222]}},\"refDevice\":{\"type\":\"Relationship\",\"object\":\"urn:ngsi-ld:Device:somedevice\"},\"temperature\":{\"type\":\"Property\",\"value\":17.2},\"type\":\"WeatherObserved\"}")
}

func TestJSONMarshallingOfBeach(t *testing.T) {
	is := is.New(t)
	e, err := NewFromJSON([]byte(entityJSONBeach))

	is.NoErr(err)
	is.Equal(e.ID(), "urn:ngsi-ld:Beach:se:sundsvall:facilities:284")
	is.Equal(e.Type(), "Beach")

	b, err := json.Marshal(e)

	is.NoErr(err)
	is.Equal(string(b), "{\"@context\":[\"https://schema.lab.fiware.org/ld/context\",\"https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld\"],\"dateCreated\":{\"type\":\"Property\",\"value\":{\"@type\":\"DateTime\",\"@value\":\"2018-06-21T15:12:39Z\"}},\"dateModified\":{\"type\":\"Property\",\"value\":{\"@type\":\"DateTime\",\"@value\":\"2021-12-17T16:34:33Z\"}},\"description\":{\"type\":\"Property\",\"value\":\"Hartungvikens havsbad är en stor härlig badstrand på östra sidan av Alnön. Badet har grillplats, omklädningsrum, WC och stora ytor för exempelvis beachvolley. Vid fint väder kan det vara svårt att hitta parkeringsplats. Öppet maj-augusti. Vattenprover tas.\"},\"id\":\"urn:ngsi-ld:Beach:se:sundsvall:facilities:284\",\"location\":{\"type\":\"GeoProperty\",\"value\":{\"type\":\"MultiPolygon\",\"coordinates\":[[[[17.520241628594867,62.39202078116761],[17.519064228331278,62.39193747751045],[17.518323811810244,62.39168934437004],[17.518020723488036,62.39159021514352],[17.517291113415897,62.39178044225612],[17.517051435725556,62.39150715425749],[17.517775420205343,62.391353241580134],[17.517946631701165,62.39112080605375],[17.51784264059981,62.39070826664903],[17.5180078905702,62.390508126725614],[17.51869745499262,62.390564053419126],[17.519787564311336,62.39064089214144],[17.520719447455495,62.391207431194566],[17.520371138161703,62.39201441340692],[17.520241628594867,62.39202078116761]]]]}},\"name\":{\"type\":\"Property\",\"value\":\"Hartungviken\"},\"refSeeAlso\":{\"type\":\"Relationship\",\"object\":[\"urn:ngsi-ld:Device:se:servanet:lora:sk-elt-temp-28\",\"https://badplatsen.havochvatten.se/badplatsen/karta/#/bath/SE0712281000003472\",\"https://www.wikidata.org/wiki/Q680645\"]},\"type\":\"Beach\"}")
}

func TestJSONMarshallingOfExerciseTrail(t *testing.T) {
	is := is.New(t)
	e, err := NewFromJSON([]byte(entityJSONExerciseTrail))

	is.NoErr(err)
	is.Equal(e.ID(), "urn:ngsi-ld:ExerciseTrail:se:sundsvall:facilities:650")
	is.Equal(e.Type(), "ExerciseTrail")

	b, err := json.Marshal(e)

	is.NoErr(err)
	is.Equal(string(b), "{\"@context\":[\"https://schema.lab.fiware.org/ld/context\",\"https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld\"],\"areaServed\":{\"type\":\"Property\",\"value\":\"Motionsspår Södra spårområdet\"},\"category\":{\"type\":\"Property\",\"value\":[\"floodlit\",\"ski-classic\",\"ski-skate\"]},\"dateCreated\":{\"type\":\"Property\",\"value\":{\"@type\":\"DateTime\",\"@value\":\"2019-01-23T09:19:21Z\"}},\"dateModified\":{\"type\":\"Property\",\"value\":{\"@type\":\"DateTime\",\"@value\":\"2022-04-19T21:07:14Z\"}},\"description\":{\"type\":\"Property\",\"value\":\"Motionsspår med 3 meter bred asfalt för rullskidor, samt 1,5 meter bred grusbädd för promenad/löpning/cykling. Vintertid enbart skidåkning, med 3 meter skateyta och dubbla klassiska spår. Konstsnöbeläggs.\"},\"id\":\"urn:ngsi-ld:ExerciseTrail:se:sundsvall:facilities:650\",\"length\":{\"type\":\"Property\",\"value\":0.9},\"location\":{\"type\":\"GeoProperty\",\"value\":{\"type\":\"LineString\",\"coordinates\":[[17.308707161238566,62.36635873125322],[17.30876459011519,62.36642793916341],[17.30877068643981,62.36653082743534],[17.308721385538732,62.36660862404391],[17.308607388457748,62.36666251579953],[17.308441362274635,62.36669416477839],[17.308383278787474,62.366693887023146],[17.306905591601655,62.36658642682947],[17.306088435388467,62.36639658586869],[17.305201661441256,62.36618008501608],[17.30502861396609,62.366122074745334],[17.30502861396609,62.366122074745334],[17.304896742172204,62.36602282594829],[17.304828502603023,62.36597369228551],[17.304691882507868,62.36592057164345],[17.30449499509765,62.36586779494021],[17.30449499509765,62.36586779494021],[17.304302290990343,62.36583803887086],[17.304129945488526,62.36580719093905],[17.30410319227664,62.36580315495848],[17.30395491061922,62.3657770254668],[17.30379513209931,62.36575769825618],[17.30344527500855,62.36572204518083],[17.30344527500855,62.36572204518083],[17.30319332372897,62.36568077306101],[17.303047988855617,62.36562508574742],[17.302889490653907,62.365538743599004],[17.302651749182843,62.36534695647898],[17.30241674122415,62.365163692099486],[17.30235195608249,62.36511443901263],[17.30234202221011,62.36507963335273],[17.30234202221011,62.36507963335273],[17.302348877252165,62.36497025417699],[17.302388693902092,62.36490612148659],[17.30256109730512,62.364749694645496],[17.30256109730512,62.364749694645496],[17.30273305732357,62.36454799197029],[17.30285380445705,62.36439857302913],[17.302945357263944,62.36432913194809],[17.303097886569986,62.36428118126726],[17.303097886569986,62.36428118126726],[17.303135432948952,62.36427859402063],[17.303292660498347,62.364283964210536],[17.30329790958957,62.36428730152742],[17.303378302699496,62.36429160991189],[17.303378302699496,62.36429160991189],[17.303461225153846,62.36439195589102],[17.303483835131107,62.364482747468024],[17.30344575612861,62.364707526261206],[17.30344804157157,62.36476635211543],[17.3034454836833,62.3648090489812],[17.303441000521488,62.36488388271841],[17.30345766993305,62.36506007989409],[17.303503686013542,62.36517246979528],[17.30363479781367,62.365287861851215],[17.30436609357238,62.365658379578434],[17.30484009136738,62.36583781784463],[17.305006977650073,62.36589252607683],[17.305228030466434,62.36595310492681],[17.305228030466434,62.36595310492681],[17.30537836882812,62.36599224162196],[17.305508342616786,62.36602471881885],[17.30560852883784,62.36603829715219],[17.30560852883784,62.36603829715219],[17.306496957700258,62.36613481075285],[17.307302228555194,62.3662137303031],[17.307725557731104,62.36625201924858],[17.30823067748755,62.36629122941522],[17.308618659480413,62.3663292735882],[17.308707161238566,62.36635873125322]]}},\"name\":{\"type\":\"Property\",\"value\":\"Motion 1 km Kallaspåret\"},\"source\":{\"type\":\"Property\",\"value\":\"https://api.sundsvall.se/facilities/2.1/get/650\"},\"status\":{\"type\":\"Property\",\"value\":\"closed\"},\"type\":\"ExerciseTrail\"}")
}

func TestRemoveAttribute(t *testing.T) {
	is := is.New(t)
	e, err := NewFromJSON([]byte(entityJSON))
	is.NoErr(err)

	impl, ok := e.(*EntityImpl)
	is.True(ok)
	is.Equal(2, len(impl.properties))

	impl.RemoveAttribute(func(attributeType, attributeName string, contents any) bool {
		return attributeName == `temperature`
	})
	is.Equal(1, len(impl.properties))
}

var entityJSON string = `{
    "id": "urn:ngsi-ld:WeatherObserved:observationid",
    "type": "WeatherObserved",
    "location": {
        "type": "GeoProperty",
        "value": {
            "type": "Point",
            "coordinates": [-8.768460000000001, 42.60214472222222]
        }
    },
	"refDevice": {
		"type": "Relationship",
		"object": "urn:ngsi-ld:Device:somedevice" 
	},
	"temperature": {
		"type": "Property",
		"value": 17.2
	},
    "@context": [
        "https://schema.lab.fiware.org/ld/context",
        "https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld"
    ]
}`

var entityJSONBeach string = `{
    "@context": [
      "https://schema.lab.fiware.org/ld/context",
      "https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld"
    ],
    "dateCreated": {
      "type": "Property",
      "value": {
        "@type": "DateTime",
        "@value": "2018-06-21T15:12:39Z"
      }
    },
    "dateModified": {
      "type": "Property",
      "value": {
        "@type": "DateTime",
        "@value": "2021-12-17T16:34:33Z"
      }
    },
    "description": {
      "type": "Property",
      "value": "Hartungvikens havsbad är en stor härlig badstrand på östra sidan av Alnön. Badet har grillplats, omklädningsrum, WC och stora ytor för exempelvis beachvolley. Vid fint väder kan det vara svårt att hitta parkeringsplats. Öppet maj-augusti. Vattenprover tas."
    },
    "id": "urn:ngsi-ld:Beach:se:sundsvall:facilities:284",
    "location": {
      "type": "GeoProperty",
      "value": {
        "coordinates": [
          [
            [
              [
                17.520241628594867,
                62.39202078116761
              ],
              [
                17.519064228331278,
                62.39193747751045
              ],
              [
                17.518323811810244,
                62.39168934437004
              ],
              [
                17.518020723488036,
                62.39159021514352
              ],
              [
                17.517291113415897,
                62.39178044225612
              ],
              [
                17.517051435725556,
                62.39150715425749
              ],
              [
                17.517775420205343,
                62.391353241580134
              ],
              [
                17.517946631701165,
                62.39112080605375
              ],
              [
                17.51784264059981,
                62.39070826664903
              ],
              [
                17.5180078905702,
                62.390508126725614
              ],
              [
                17.51869745499262,
                62.390564053419126
              ],
              [
                17.519787564311336,
                62.39064089214144
              ],
              [
                17.520719447455495,
                62.391207431194566
              ],
              [
                17.520371138161703,
                62.39201441340692
              ],
              [
                17.520241628594867,
                62.39202078116761
              ]
            ]
          ]
        ],
        "type": "MultiPolygon"
      }
    },
    "name": {
      "type": "Property",
      "value": "Hartungviken"
    },
    "refSeeAlso": {
      "object": [
        "urn:ngsi-ld:Device:se:servanet:lora:sk-elt-temp-28",
        "https://badplatsen.havochvatten.se/badplatsen/karta/#/bath/SE0712281000003472",
        "https://www.wikidata.org/wiki/Q680645"
      ],
      "type": "Relationship"
    },
    "type": "Beach"
  }`

var entityJSONExerciseTrail string = `{
    "@context": [
      "https://schema.lab.fiware.org/ld/context",
      "https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld"
    ],
    "areaServed": {
      "type": "Property",
      "value": "Motionsspår Södra spårområdet"
    },
    "category": {
      "type": "Property",
      "value": [
        "floodlit",
        "ski-classic",
        "ski-skate"
      ]
    },
    "dateCreated": {
      "type": "Property",
      "value": {
        "@type": "DateTime",
        "@value": "2019-01-23T09:19:21Z"
      }
    },
    "dateModified": {
      "type": "Property",
      "value": {
        "@type": "DateTime",
        "@value": "2022-04-19T21:07:14Z"
      }
    },
    "description": {
      "type": "Property",
      "value": "Motionsspår med 3 meter bred asfalt för rullskidor, samt 1,5 meter bred grusbädd för promenad/löpning/cykling. Vintertid enbart skidåkning, med 3 meter skateyta och dubbla klassiska spår. Konstsnöbeläggs."
    },
    "id": "urn:ngsi-ld:ExerciseTrail:se:sundsvall:facilities:650",
    "length": {
      "type": "Property",
      "value": 0.9
    },
    "location": {
      "type": "GeoProperty",
      "value": {
        "coordinates": [
          [
            17.308707161238566,
            62.36635873125322
          ],
          [
            17.30876459011519,
            62.36642793916341
          ],
          [
            17.30877068643981,
            62.36653082743534
          ],
          [
            17.308721385538732,
            62.36660862404391
          ],
          [
            17.308607388457748,
            62.36666251579953
          ],
          [
            17.308441362274635,
            62.36669416477839
          ],
          [
            17.308383278787474,
            62.366693887023146
          ],
          [
            17.306905591601655,
            62.36658642682947
          ],
          [
            17.306088435388467,
            62.36639658586869
          ],
          [
            17.305201661441256,
            62.36618008501608
          ],
          [
            17.30502861396609,
            62.366122074745334
          ],
          [
            17.30502861396609,
            62.366122074745334
          ],
          [
            17.304896742172204,
            62.36602282594829
          ],
          [
            17.304828502603023,
            62.36597369228551
          ],
          [
            17.304691882507868,
            62.36592057164345
          ],
          [
            17.30449499509765,
            62.36586779494021
          ],
          [
            17.30449499509765,
            62.36586779494021
          ],
          [
            17.304302290990343,
            62.36583803887086
          ],
          [
            17.304129945488526,
            62.36580719093905
          ],
          [
            17.30410319227664,
            62.36580315495848
          ],
          [
            17.30395491061922,
            62.3657770254668
          ],
          [
            17.30379513209931,
            62.36575769825618
          ],
          [
            17.30344527500855,
            62.36572204518083
          ],
          [
            17.30344527500855,
            62.36572204518083
          ],
          [
            17.30319332372897,
            62.36568077306101
          ],
          [
            17.303047988855617,
            62.36562508574742
          ],
          [
            17.302889490653907,
            62.365538743599004
          ],
          [
            17.302651749182843,
            62.36534695647898
          ],
          [
            17.30241674122415,
            62.365163692099486
          ],
          [
            17.30235195608249,
            62.36511443901263
          ],
          [
            17.30234202221011,
            62.36507963335273
          ],
          [
            17.30234202221011,
            62.36507963335273
          ],
          [
            17.302348877252165,
            62.36497025417699
          ],
          [
            17.302388693902092,
            62.36490612148659
          ],
          [
            17.30256109730512,
            62.364749694645496
          ],
          [
            17.30256109730512,
            62.364749694645496
          ],
          [
            17.30273305732357,
            62.36454799197029
          ],
          [
            17.30285380445705,
            62.36439857302913
          ],
          [
            17.302945357263944,
            62.36432913194809
          ],
          [
            17.303097886569986,
            62.36428118126726
          ],
          [
            17.303097886569986,
            62.36428118126726
          ],
          [
            17.303135432948952,
            62.36427859402063
          ],
          [
            17.303292660498347,
            62.364283964210536
          ],
          [
            17.30329790958957,
            62.36428730152742
          ],
          [
            17.303378302699496,
            62.36429160991189
          ],
          [
            17.303378302699496,
            62.36429160991189
          ],
          [
            17.303461225153846,
            62.36439195589102
          ],
          [
            17.303483835131107,
            62.364482747468024
          ],
          [
            17.30344575612861,
            62.364707526261206
          ],
          [
            17.30344804157157,
            62.36476635211543
          ],
          [
            17.3034454836833,
            62.3648090489812
          ],
          [
            17.303441000521488,
            62.36488388271841
          ],
          [
            17.30345766993305,
            62.36506007989409
          ],
          [
            17.303503686013542,
            62.36517246979528
          ],
          [
            17.30363479781367,
            62.365287861851215
          ],
          [
            17.30436609357238,
            62.365658379578434
          ],
          [
            17.30484009136738,
            62.36583781784463
          ],
          [
            17.305006977650073,
            62.36589252607683
          ],
          [
            17.305228030466434,
            62.36595310492681
          ],
          [
            17.305228030466434,
            62.36595310492681
          ],
          [
            17.30537836882812,
            62.36599224162196
          ],
          [
            17.305508342616786,
            62.36602471881885
          ],
          [
            17.30560852883784,
            62.36603829715219
          ],
          [
            17.30560852883784,
            62.36603829715219
          ],
          [
            17.306496957700258,
            62.36613481075285
          ],
          [
            17.307302228555194,
            62.3662137303031
          ],
          [
            17.307725557731104,
            62.36625201924858
          ],
          [
            17.30823067748755,
            62.36629122941522
          ],
          [
            17.308618659480413,
            62.3663292735882
          ],
          [
            17.308707161238566,
            62.36635873125322
          ]
        ],
        "type": "LineString"
      }
    },
    "name": {
      "type": "Property",
      "value": "Motion 1 km Kallaspåret"
    },
    "source": {
      "type": "Property",
      "value": "https://api.sundsvall.se/facilities/2.1/get/650"
    },
    "status": {
      "type": "Property",
      "value": "closed"
    },
    "type": "ExerciseTrail"
  }`
