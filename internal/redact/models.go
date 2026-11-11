package redact

// TokenclsModelInfo describes a downloadable local ONNX token-classification
// model. The files are hosted on the aigc-cli-models release and installed under
// models/redact/<id>/.
type TokenclsModelInfo struct {
	ID          string
	Name        string
	Description string
	License     string
	Source      string
	Files       []TokenclsModelFile
}

// TokenclsModelFile is one downloadable file in a tokencls model pack.
type TokenclsModelFile struct {
	URL     string
	OutName string
	SizeMB  int64
	SHA256  string
}

const tokenclsModelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// TokenclsModels returns the available local ONNX model packs.
func TokenclsModels() []TokenclsModelInfo {
	return []TokenclsModelInfo{
		{
			ID:          "zh-pii",
			Name:        "privyscope zh-Hans",
			Description: "Chinese PII token classifier (PER/PHONE/ID/EMAIL/ADDRESS/BANK/DATE/SECRET)",
			License:     "Apache-2.0",
			Source:      "https://github.com/zafrem/privyscope-zh-hans",
			Files: []TokenclsModelFile{
				{URL: tokenclsModelsBaseURL + "/redact-zh-pii-model.onnx", OutName: "model.onnx", SizeMB: 98,
					SHA256: "2edf270a23f9a989c9a6e74dbcc941784fb3410b80fd9de9a7409dc5fce4be2e"},
				{URL: tokenclsModelsBaseURL + "/redact-zh-pii-tokenizer.json", OutName: "tokenizer.json", SizeMB: 1,
					SHA256: "7dfbf1966ebf99d471c3796e9b457329d2b2182b817e144f1e904b957745c839"},
				{URL: tokenclsModelsBaseURL + "/redact-zh-pii-config.json", OutName: "config.json", SizeMB: 0,
					SHA256: "ebc47b532f7a36568061ad7a9580793f442ad151d61fa53eddb4b965d0700649"},
				{URL: tokenclsModelsBaseURL + "/redact-zh-pii-LICENSE.txt", OutName: "LICENSE.txt", SizeMB: 0,
					SHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4"},
				{URL: tokenclsModelsBaseURL + "/redact-zh-pii-NOTICE.txt", OutName: "NOTICE.txt", SizeMB: 0,
					SHA256: "a32b34975c89984d73fde7b00006012c8671b73b0e491c3aea63b4325c743207"},
			},
		},
		{
			ID:          "multilingual-ner",
			Name:        "Davlan mBERT multilingual NER",
			Description: "Chinese+English PER/ORG/LOC/DATE token classifier (10 languages)",
			License:     "AFL-3.0",
			Source:      "https://github.com/martianzhang/aigc-cli-models/releases/tag/v1",
			Files: []TokenclsModelFile{
				{URL: tokenclsModelsBaseURL + "/redact-multilingual-ner-model.onnx", OutName: "model.onnx", SizeMB: 171,
					SHA256: "1213fdd405d295768b0d41d8214062f2f278f0e3acff6af67d8fd47360d2be0f"},
				{URL: tokenclsModelsBaseURL + "/redact-multilingual-ner-tokenizer.json", OutName: "tokenizer.json", SizeMB: 3,
					SHA256: "bf1b59b7b11c95f194f51708d918eea378e09d05f84c0e1656dc5180e8117088"},
				{URL: tokenclsModelsBaseURL + "/redact-multilingual-ner-config.json", OutName: "config.json", SizeMB: 0,
					SHA256: "8e5caefadaf9923a9e7d3de42ca97780c68fc4d83519d333f141b299e40af638"},
				{URL: tokenclsModelsBaseURL + "/redact-multilingual-ner-LICENSE.txt", OutName: "LICENSE.txt", SizeMB: 0,
					SHA256: "de0612c5780d0d21f9982d2cadfd715d2d3b7a0dfeb532a5023d42a1d4fdce5c"},
				{URL: tokenclsModelsBaseURL + "/redact-multilingual-ner-NOTICE.txt", OutName: "NOTICE.txt", SizeMB: 0,
					SHA256: "079b75d8f9e4291013249b8e072869f4960ebf4849e24e78e48f33d0b765015f"},
			},
		},
	}
}

// FindTokenclsModel returns the model pack with the given ID.
func FindTokenclsModel(id string) (TokenclsModelInfo, bool) {
	for _, m := range TokenclsModels() {
		if m.ID == id {
			return m, true
		}
	}
	return TokenclsModelInfo{}, false
}
