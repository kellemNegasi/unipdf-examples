/*
 * This example showcases how to append a new page with signature to a PDF document.
 * The file is signed using a private/public key pair.
 *
 * $ ./pdf_sign_encrypted_pdf <INPUT_PDF_PATH> <OUTPUT_PDF_PATH>
 */
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/unidoc/unipdf/v4/common/license"
	"github.com/unidoc/unipdf/v4/core/security"

	"github.com/unidoc/unipdf/v4/annotator"
	"github.com/unidoc/unipdf/v4/core"
	"github.com/unidoc/unipdf/v4/model"
	"github.com/unidoc/unipdf/v4/model/sighandler"
)

func init() {
	//Make sure to load your metered License API key prior to using the library.
	//If you need a key, you can sign up and create a free one at https://cloud.unidoc.io
	err := license.SetMeteredKey(os.Getenv(`UNIDOC_LICENSE_API_KEY`))
	if err != nil {
		panic(err)
	}
}

func main() {
	args := os.Args
	if len(args) < 3 {
		log.Fatalln("Usage: go run pdf_sign_encrypted_pdf INPUT_PDF_PATH OUTPUT_PDF_PATH")
	}

	inputPath := args[1]
	outputPath := args[2]

	// Read the original file.
	f, err := os.Open(inputPath)
	if err != nil {
		log.Fatalf("Fail: %v\n", err)
	}
	defer f.Close()

	pdfReader, err := model.NewPdfReader(f)
	if err != nil {
		log.Fatalf("Fail: %v\n", err)
	}

	password := "password"

	permissions := security.PermPrinting | // Allow printing with low quality
		security.PermFullPrintQuality |
		security.PermModify | // Allow modifications.
		security.PermAnnotate | // Allow annotations.
		security.PermFillForms |
		security.PermRotateInsert | // Allow modifying page order, rotating pages etc.
		security.PermExtractGraphics | // Allow extracting graphics.
		security.PermDisabilityExtract // Allow extracting graphics (accessibility)

	encryptOptions := &model.EncryptOptions{
		Permissions: permissions,
		Algorithm:   model.AES_128bit,
	}

	// Ecnrypt document
	buf, err := encryptDocument(pdfReader, password, encryptOptions)
	if err != nil {
		log.Fatalf("Fail: %v\n", err)
	}

	readerOpts := model.NewReaderOpts()
	readerOpts.Password = password

	// Open reader for the signed document
	pdfReader, err = model.NewPdfReaderWithOpts(bytes.NewReader(buf), readerOpts)
	if err != nil {
		log.Fatalf("Fail: %v\n", err)
	}

	// Create appender.
	pdfAppender, err := model.NewPdfAppenderWithOpts(pdfReader, readerOpts, encryptOptions)
	if err != nil {
		log.Fatalf("Fail: %v\n", err)
	}

	// Add signature
	buf, err = addSignature(pdfAppender, 1)
	if err != nil {
		log.Fatalf("Fail: %v\n", err)
	}

	// Write the resulting file to output.pdf file.
	err = ioutil.WriteFile(outputPath, buf, 0666)
	if err != nil {
		log.Fatalf("Fail: %v\n", err)
	}
	log.Printf("PDF file successfully saved to output path: %s\n", outputPath)

	fmt.Println("Done")
}

func encryptDocument(pdfReader *model.PdfReader, password string, encryptOptions *model.EncryptOptions) ([]byte, error) {
	pdfWriter, err := pdfReader.ToWriter(nil)
	if err != nil {
		return nil, err
	}

	// Encrypt document before writing to file.
	err = pdfWriter.Encrypt([]byte(password), []byte(password), encryptOptions)
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	// Write output PDF file.
	if err = pdfWriter.Write(buf); err != nil {
		return nil, err
	}

	log.Println("PDF file successfully encrypted")

	return buf.Bytes(), nil
}

func addSignature(pdfAppender *model.PdfAppender, pageNum int) ([]byte, error) {
	// Generate key pair.
	priv, cert, err := generateSigKeys()
	if err != nil {
		return nil, err
	}

	// Create signature handler.
	handler, err := sighandler.NewAdobePKCS7Detached(priv, cert)
	if err != nil {
		return nil, err
	}

	// Create signature.
	signature := model.NewPdfSignature(handler)
	signature.SetName("Test Signature Appearance Name")
	signature.SetReason("TestSignatureAppearance Reason")
	signature.SetDate(time.Now(), "")

	// Initialize signature.
	if err := signature.Initialize(); err != nil {
		return nil, err
	}

	opts := annotator.NewSignatureFieldOpts()
	opts.FontSize = 8
	opts.Rect = []float64{float64(50), 250, float64(150), 300}
	opts.TextColor = model.NewPdfColorDeviceRGB(255, 0, 0)

	sigField, err := annotator.NewSignatureField(
		signature,
		[]*annotator.SignatureLine{
			annotator.NewSignatureLine("Name", "John Doe"),
			annotator.NewSignatureLine("Date", "2019.03.14"),
			annotator.NewSignatureLine("Reason", fmt.Sprintf("Test sign")),
			annotator.NewSignatureLine("Location", "London"),
			annotator.NewSignatureLine("DN", "authority2:name2"),
		},
		opts,
	)
	if err != nil {
		return nil, err
	}

	sigField.T = core.MakeString(fmt.Sprintf("New Page Signature"))

	if err = pdfAppender.Sign(pageNum, sigField); err != nil {
		log.Fatalf("Fail: %v\n", err)
	}

	buf := &bytes.Buffer{}
	// Write output PDF file.
	if err = pdfAppender.Write(buf); err != nil {
		return nil, err
	}

	log.Println("PDF file successfully signed")

	return buf.Bytes(), nil
}

func generateSigKeys() (*rsa.PrivateKey, *x509.Certificate, error) {
	var now = time.Now()

	// Generate private key.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Initialize X509 certificate template.
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "any",
			Organization: []string{"Test Company"},
		},
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(time.Hour * 24 * 365),

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Generate X509 certificate.
	certData, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, nil, err
	}

	return priv, cert, nil
}
