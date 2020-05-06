package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Domain struct {
	domain         string
	topLevelDomain string
}

var projectId, credentialsJSON, cName string
var domains []string

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("Error loading .env file. %v\n", err)
	}

	projectId = getEnvOrFail("PROJECT_ID")
	credentialsJSON = getEnvOrFail("CREDENTIALS_JSON")
	cName = strings.TrimSpace(getEnvOrFail("CNAME"))
	domains = strings.Fields(getEnvOrFail("DOMAINS"))
	port, ok := os.LookupEnv("PORT")
	if ok == false {
		port = "8080"
	}

	http.HandleFunc("/check-and-refresh-entries", CheckAndRefreshEntries)
	fmt.Printf("Starting server on port :%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Error Listening on Port. %v\n", err)
	}
}

func CheckAndRefreshEntries(w http.ResponseWriter, _ *http.Request) {

	cNameIp, err := lookupIP(cName)
	if err != nil {
		log.Fatal("could not get IP for cName", err)
	}

	wrongDomains := getWrongDomains(cNameIp)

	if len(wrongDomains) <= 0 {
		fmt.Printf("No wrong Domains")
		_, _ = fmt.Fprintf(w, "No Wrong Domains")
		return
	}

	ctx := context.Background()
	service, err := dns.NewService(ctx, option.WithCredentialsJSON([]byte(credentialsJSON)))

	if err != nil {
		log.Fatal("could not create dns service", err)
	}

	getManagedZones, err := service.ManagedZones.List(projectId).Do()
	if err != nil {
		log.Fatal("could not get managed zones ", err)
	}

	for _, managedZone := range getManagedZones.ManagedZones {
		err := correctManagedZone(managedZone, wrongDomains, service, cNameIp)
		if err != nil {
			log.Fatalf("cloud not macke change to dns. ManagedZoneDns: %s, ManagedZoneId: %d", managedZone.DnsName, managedZone.Id)
		}
	}
	_, _ = fmt.Fprintf(w, "Fixed wrong domain. wrongDomains: %v", wrongDomains)
}

func correctManagedZone(managedZone *dns.ManagedZone, wrongDomains []Domain, service *dns.Service, cNameIp string) error {
	managedZoneId := strconv.FormatUint(managedZone.Id, 10)
	resourceDomain, changeDnsErr := getToplevelDomain(managedZone.DnsName)
	if changeDnsErr != nil {
		log.Fatalf("could not get resouceDomain. ManagedZoneDns: %s, ManagedZoneId: %d", managedZone.DnsName, managedZone.Id)
	}

	currentWrongDomains, currentHasWrongDomains := getCurrentWrongDomains(wrongDomains, resourceDomain)

	if currentHasWrongDomains == false {
		return nil
	}

	resourceRecordSets, changeDnsErr := service.ResourceRecordSets.List(projectId, managedZoneId).Do()
	if changeDnsErr != nil {
		log.Fatalf("could not get resource record set. ManagedZoneDns: %s, ManagedZoneId: %d", managedZone.DnsName, managedZone.Id)
	}

	additions, deletions := getAdditionsAndDeletions(resourceRecordSets, currentWrongDomains, cNameIp)

	if len(additions) <= 0 {
		return nil
	}

	dnsChange := &dns.Change{
		Additions: additions,
		Deletions: deletions,
	}

	_, err := service.Changes.Create(projectId, managedZoneId, dnsChange).Do()
	return err
}

func getWrongDomains(cNameIp string) []Domain {
	var wrongDomains []Domain
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		lookupDomain := getLookupDomain(domain)

		domainIp, err := lookupIP(lookupDomain)
		if err != nil {
			log.Fatal("Could not get IP for domain: "+domain, err)
		}

		if cNameIp == domainIp == false {
			topLevelDomain, err := getToplevelDomain(domain)
			if err != nil {
				log.Fatalf("Could not get top level of domain. domain: %s", domain)
			}

			wrongDomains = append(wrongDomains, Domain{
				domain:         domain,
				topLevelDomain: topLevelDomain,
			})
		}
	}
	return wrongDomains
}

func getAdditionsAndDeletions(resourceRecordSets *dns.ResourceRecordSetsListResponse, currentWrongDomains map[string]bool, cNameIp string) ([]*dns.ResourceRecordSet, []*dns.ResourceRecordSet) {
	var additions []*dns.ResourceRecordSet
	{
	}
	var deletions []*dns.ResourceRecordSet
	{
	}

	for _, rSet := range resourceRecordSets.Rrsets {
		if rSet.Type != "A" {
			continue
		}
		rSetDomain := strings.TrimSuffix(rSet.Name, ".")
		rSetIp := rSet.Rrdatas[0]
		// if ip was already changed in the config but not propagated
		if currentWrongDomains[rSetDomain] == false || cNameIp == rSetIp {
			continue
		}

		additions = append(additions, generateResourceRecordSet(rSetDomain, cNameIp))
		deletions = append(deletions, generateResourceRecordSet(rSetDomain, rSetIp))
	}
	return additions, deletions
}

func getCurrentWrongDomains(wrongDomains []Domain, resourceDomain string) (map[string]bool, bool) {
	currentWrongDomains := make(map[string]bool)
	currentHasWrongDomains := false
	for _, wrongDomain := range wrongDomains {
		if wrongDomain.topLevelDomain == resourceDomain {
			currentHasWrongDomains = true
			currentWrongDomains[wrongDomain.domain] = true
		}
	}
	return currentWrongDomains, currentHasWrongDomains
}

func generateResourceRecordSet(domain string, ip string) *dns.ResourceRecordSet {
	return &dns.ResourceRecordSet{
		Name: domain + ".",
		Rrdatas: []string{
			ip,
		},
		Ttl:  300,
		Type: "A",
	}
}

func getEnvOrFail(envName string) string {
	envValue, ok := os.LookupEnv(envName)
	if ok == false {
		log.Fatalf("Could not load enviroment variable. EnvName: %s\n", envName)
	}
	return envValue
}

func getLookupDomain(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return "this-domain-should-not-exist" + strings.TrimPrefix(domain, "*")
	}
	return domain
}

func getToplevelDomain(domain string) (string, error) {
	domain = strings.TrimSuffix(domain, ".")
	topLevelDomainRegex := regexp.MustCompile("\\w+\\.\\w+$")
	match := topLevelDomainRegex.FindStringSubmatch(domain)
	if len(match) <= 0 {
		return "", errors.New("could not get toplevel domain")
	}
	return match[0], nil
}

func lookupIP(domain string) (string, error) {
	for retry := 0; retry < 5; retry++ {
		lookedUpIPs, err := net.LookupIP(domain)
		if err == nil && len(lookedUpIPs) > 0 {
			return lookedUpIPs[0].String(), nil
		}
		fmt.Printf("Failed to lookup ip for domain: %s, retryCount: %d, err: %v", domain, retry, err)
		time.Sleep(100 * time.Millisecond)
	}
	return "", errors.New("Could not get IP for domain: " + domain)

}
