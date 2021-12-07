package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/miekg/dns"
)

func A(fqdn, serverAddr string) ([]string, error) {
	var m dns.Msg
	var ips []string
	m.SetQuestion(dns.Fqdn(fqdn), dns.TypeA)
	i, err := dns.Exchange(&m, serverAddr)
	if err != nil {
		return ips, err
	}
	if len(i.Answer) < 1 {
		return ips, errors.New("Sem resposta.")
	}
	for _, answer := range i.Answer {
		if a, ok := answer.(*dns.A); ok {
			ips = append(ips, a.A.String())
		}
	}
	return ips, nil
}

func cname(fqdn, serverAddr string) ([]string, error) {
	var m dns.Msg
	var fqdns []string
	m.SetQuestion(dns.Fqdn(fqdn), dns.TypeCNAME)
	i, err := dns.Exchange(&m, serverAddr)
	if err != nil {
		return fqdns, err
	}
	if len(i.Answer) < 1 {
		return fqdns, errors.New("Sem resposta.")
	}
	for _, answer := range i.Answer {
		if c, ok := answer.(*dns.CNAME); ok {
			fqdns = append(fqdns, c.Target)
		}
	}
	return fqdns, nil
}

func lookup(fqdn, serverAddr string) []result {
	var results []result
	var cfqdn = fqdn
	for {
		cnames, err := cname(cfqdn, serverAddr)
		if err == nil && len(cnames) > 0 {
			cfqdn = cnames[0]
			continue
		}
		ips, err := A(cfqdn, serverAddr)
		if err != nil {
			break
		}
		for _, ip := range ips {
			results = append(results, result{IPAddr: ip, Hostname: fqdn})
		}
		break
	}
	return results
}

func work(tracker chan empty, fqdns chan string, gather chan []result, serverAddr string) {
	for fqdn := range fqdns {
		results := lookup(fqdn, serverAddr)
		if len(results) > 0 {
			gather <- results
		}
	}
	var em empty
	tracker <- em

}

type empty struct{}

type result struct {
	IPAddr   string
	Hostname string
}

func main() {
	var (
		Domain         = flag.String("d", "", "Domínio")
		Wordlist       = flag.String("w", "", "Wordlist que será usada")
		contadorWorker = flag.Int("c", 100, "Threads")
		serverDns      = flag.String("s", "8.8.8.8:53", "Server DNS usado")
	)
	flag.Parse()

	if *Domain == "" || *Wordlist == "" {
		fmt.Println("-d e -w são obrigatórios.")
		fmt.Println("Modo de uso: ./guesser -d domain -w wordlist -c threads")
	}

	var results []result

	fqdns := make(chan string, *contadorWorker)
	gather := make(chan []result)
	tracker := make(chan empty)

	fh, _ := os.Open(*Wordlist)
	defer fh.Close()
	scanner := bufio.NewScanner(fh)

	for i := 0; i < *contadorWorker; i++ {
		go work(tracker, fqdns, gather, *serverDns)
	}

	go func() {
		for r := range gather {
			results = append(results, r...)
		}
		var em empty
		tracker <- em
	}()

	for scanner.Scan() {
		fqdns <- fmt.Sprintf("%s.%s", scanner.Text(), *Domain)
	}

	close(fqdns)
	for i := 0; i < *contadorWorker; i++ {
		<-tracker
	}
	close(gather)
	<-tracker

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 4, ' ', 0)
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\n", r.Hostname, r.IPAddr)
	}
	w.Flush()
}
