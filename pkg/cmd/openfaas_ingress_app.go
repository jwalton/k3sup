package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"text/template"

	"github.com/spf13/cobra"
)

type InputData struct {
	IngressDomain    string
	CertmanagerEmail string
}

func makeInstallOpenFaaSIngress() *cobra.Command {
	var openfaasIngress = &cobra.Command{
		Use:          "openfaas-ingress",
		Short:        "Install openfaas ingress with TLS",
		Long:         `Install openfaas ingress. Requires cert-manager 0.11.0 or higher installation in the cluster. Please set --domain to your custom domain and set --email to your email - this email is used by letsencrypt for domain expiry etc.`,
		Example:      `  k3sup app install openfaas-ingress --domain openfaas.example.com --email openfaas@example.com`,
		SilenceUsage: true,
	}

	openfaasIngress.Flags().StringP("domain", "d", "", "Custom Ingress Domain")
	openfaasIngress.Flags().StringP("email", "e", "", "Letsencrypt Email")

	openfaasIngress.RunE = func(command *cobra.Command, args []string) error {

		email, _ := command.Flags().GetString("email")
		domain, _ := command.Flags().GetString("domain")

		if email == "" || domain == "" {
			return errors.New("both --email and --domain flags should be set and not empty, please set these values")
		}

		kubeConfigPath := getDefaultKubeconfig()

		if command.Flags().Changed("kubeconfig") {
			kubeConfigPath, _ = command.Flags().GetString("kubeconfig")
		}

		fmt.Printf("Using kubeconfig: %s\n", kubeConfigPath)

		yamlBytes, templateErr := buildYaml(domain, email)
		if templateErr != nil {
			log.Print("Unable to install the application. Could not build the templated yaml file for the resources")
			return templateErr
		}

		tempFile, tempFileErr := writeTempFile(yamlBytes)
		if tempFileErr != nil {
			log.Print("Unable to save generated yaml file into the temporary directory")
			return tempFileErr
		}

		res, err := kubectlTask("apply", "-f", tempFile)

		if err != nil {
			log.Print(err)
			return err
		}

		if res.Stderr != "" {
			log.Printf("Unable to install this application. Have you got OpenFaaS running in the openfaas namespace and cert-manager 0.11.0 or higher installed in cert-manager namespace? %s", res.Stderr)
			return err
		}

		fmt.Println(`=======================================================================
= OpenFaaS Ingress and cert-manager ClusterIssuer have been installed  =
=======================================================================

# You will need to ensure that your domain points to your cluster and is
# accessible through ports 80 and 443. 
#
# This is used to validate your ownership of this domain by LetsEncrypt
# and then you can use https with your installation. 

# Ingress to your domain has been installed for OpenFaaS
# to see the ingress record run
kubectl get -n openfaas ingress openfaas-gateway

# Check the cert-manager logs with:
kubectl logs -n cert-manager deploy/cert-manager

# A cert-manager ClusterIssuer has been installed into the default
# namespace - to see the resource run
kubectl describe ClusterIssuer letsencrypt-prod

# To check the status of your certificate you can run
kubectl describe -n openfaas Certificate openfaas-gateway

# It may take a while to be issued by LetsEncrypt, in the meantime a 
# self-signed cert will be installed


Thank you for using k3sup!`)

		return nil
	}

	return openfaasIngress
}

func createTempDirectory(directory string) (string, error) {
	tempDirectory := filepath.Join(os.TempDir(), directory)
	if _, err := os.Stat(tempDirectory); os.IsNotExist(err) {
		log.Printf(tempDirectory)
		errr := os.Mkdir(tempDirectory, 0744)
		if errr != nil {
			log.Printf("couldnt make dir %s", err)
			return "", err
		}
	}

	return tempDirectory, nil
}

func writeTempFile(input []byte) (string, error) {
	var tempDirectory, dirErr = createTempDirectory(".k3sup/")
	if dirErr != nil {
		return "", dirErr
	}

	filename := filepath.Join(tempDirectory, "temp_openfaas_ingress.yaml")

	err := ioutil.WriteFile(filename, input, 0744)
	if err != nil {
		return "", err
	}
	return filename, nil
}

func buildYaml(domain string, email string) ([]byte, error) {
	tmpl, err := template.New("yaml").Parse(yamlTemplate)

	if err != nil {
		return nil, err
	}

	inputData := InputData{
		IngressDomain:    domain,
		CertmanagerEmail: email,
	}
	var tpl bytes.Buffer

	err = tmpl.Execute(&tpl, inputData)

	if err != nil {
		return nil, err
	}

	return tpl.Bytes(), nil
}

var yamlTemplate = `
apiVersion: extensions/v1beta1 
kind: Ingress
metadata:
  name: openfaas-gateway
  namespace: openfaas
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    kubernetes.io/ingress.class: nginx
spec:
  rules:
  - host: {{.IngressDomain}}
    http:
      paths:
      - backend:
          serviceName: gateway
          servicePort: 8080
        path: /
  tls:
  - hosts:
    - {{.IngressDomain}}
    secretName: openfaas-gateway
---
apiVersion: cert-manager.io/v1alpha2
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    email: {{.CertmanagerEmail}}
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: example-issuer-account-key
    solvers:
    - http01:
        ingress:
          class: nginx`
