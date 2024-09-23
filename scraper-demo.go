package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/PuerkitoBio/goquery"
)

type Region struct {
	Name    string
	RepID   string
	NonRepID string
}

type Quest struct {
	Name   string
	Link   string
	Region string
	Type   string // "Reputation" or "Non-Reputation"
}

func main() {
	url := "https://gamewith.net/genshin-impact/article/show/22408"

	regions := []Region{
		// {Name: "Natlan", RepID: "natWQL", NonRepID: "natNR1"},
		// {Name: "Fontaine", RepID: "fonWQL", NonRepID: "fonNR1"},
		// {Name: "Sumeru", RepID: "sumWQL", NonRepID: "numNR"},
		{Name: "Inazuma", RepID: "inaWQL", NonRepID: "zumaNR1"},
		{Name: "Liyue", RepID: "liyue1", NonRepID: "liyue2"},
		{Name: "Mondstadt", RepID: "mon1", NonRepID: "mon2"},
	}

	quests := scrapeQuests(url, regions)
	visitQuests(quests)
}

func scrapeQuests(url string, regions []Region) []Quest {
	var quests []Quest

	c := colly.NewCollector(
		colly.AllowedDomains("gamewith.net"),
		colly.Async(true),
	)

	// Disable image loading
	c.DisableCookies()
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.5")
		r.Headers.Set("Accept-Encoding", "gzip, deflate, br")
	})

	c.OnHTML("body", func(e *colly.HTMLElement) {
		time.Sleep(2 * time.Second) // Reduced wait time

		for _, region := range regions {
			quests = append(quests, scrapeRegionQuests(e, region, "Reputation")...)
			quests = append(quests, scrapeRegionQuests(e, region, "Non-Reputation")...)
		}
	})

	c.Visit(url)
	c.Wait()

	return quests
}

func scrapeRegionQuests(e *colly.HTMLElement, region Region, questType string) []Quest {
	var quests []Quest
	var title string

	if questType == "Reputation" {
		title = e.ChildText(fmt.Sprintf("#%s", region.RepID))
	} else {
		title = e.ChildText(fmt.Sprintf("#%s", region.NonRepID))
	}

	fmt.Printf("\n%s %s World Quests:\n", region.Name, questType)

	e.ForEach("#article-body h3", func(_ int, el *colly.HTMLElement) {
		if strings.Contains(el.Text, title) {
			el.DOM.Next().Find("li").Each(func(_ int, s *goquery.Selection) {
				questName := strings.TrimSpace(s.Find("a").Text())
				questLink, _ := s.Find("a").Attr("href")
				fmt.Printf("- Quest: %s\n  Link: %s\n", questName, questLink)
				quests = append(quests, Quest{
					Name:   questName,
					Link:   questLink,
					Region: region.Name,
					Type:   questType,
				})
			})
		}
	})

	return quests
}

func visitQuests(quests []Quest) {
	c := colly.NewCollector(
		colly.AllowedDomains("gamewith.net"),
	)

	// Limit the number of concurrent requests and add a delay between requests
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*gamewith.net*",
		Parallelism: 1,
		Delay:       2 * time.Second,
		RandomDelay: 1 * time.Second,
	})

	// Disable image loading
	c.DisableCookies()
	c.OnRequest(func(r *colly.Request) {
		quest := r.Ctx.GetAny("quest").(Quest)
		fmt.Printf("Visiting %s quest %s\n", quest.Region, r.URL)
	})

	c.OnHTML("body", func(e *colly.HTMLElement) {
		quest := e.Request.Ctx.GetAny("quest").(Quest)
		fmt.Printf("\n%s - %s\n", quest.Region, quest.Name)

		// Try new selector first
		steps := e.DOM.Find("div.genshin_quest > ol > li").Length()

		// If no steps found, try old selector
		if steps == 0 {
			e.DOM.Find("#article-body > table").Each(func(_ int, table *goquery.Selection) {
				rows := table.Find("tr").Length()
				if rows > steps {
					steps = rows - 1 // Subtract 1 to account for the header row
				}
			})
		}

		if steps > 0 {
			if e.DOM.Find("div.genshin_quest").Length() > 0 {
				fmt.Println("(Using new selector)")
			} else {
				fmt.Println("(Using old selector)")
			}
			fmt.Printf("Steps to Complete: %d\n", steps)
		} else {
			fmt.Println("Steps to Complete: Unknown")
		}

		fmt.Println("Rewards:")

		rewardsFound := false

		// Method 1: Find the "Rewards List" header and check the following ul
		e.DOM.Find("#article-body > h3").Each(func(_ int, s *goquery.Selection) {
			if strings.Contains(s.Text(), "Rewards List") {
				s.Next().Filter("ul").Find("li").Each(func(_ int, reward *goquery.Selection) {
					fmt.Printf("- %s\n", strings.TrimSpace(reward.Text()))
					rewardsFound = true
				})
			}
		})

		// Method 2: Check for direct ul under article-body
		if !rewardsFound {
			e.DOM.Find("#article-body > ul").Each(func(_ int, ul *goquery.Selection) {
				ul.Find("li").Each(func(_ int, reward *goquery.Selection) {
					rewardText := strings.TrimSpace(reward.Text())
					if rewardText != "" {
						fmt.Printf("- %s\n", rewardText)
						rewardsFound = true
					}
				})
			})
		}

		// Method 3: Check for table with class genshin_center
		if !rewardsFound {
			e.DOM.Find("#article-body table.genshin_center td").Each(func(_ int, reward *goquery.Selection) {
				rewardText := strings.TrimSpace(reward.Text())
				if rewardText != "" {
					fmt.Printf("- %s\n", rewardText)
					rewardsFound = true
				}
			})
		}

		// Method 4: Check for rewards in the genshin_housyu table
		if !rewardsFound {
			rewardsFound = findRewardsInHousyuTable(e)
		}

		// Method 5: Check for rewards in the table with selector #article-body > table:nth-child(34) > tbody
		if !rewardsFound {
			rewardsFound = findRewardsInSpecificTable(e)
		}

		if !rewardsFound {
			fmt.Println("No rewards found for this quest.")
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		quest := r.Ctx.GetAny("quest").(Quest)
		fmt.Printf("Error visiting %s quest %s: %s\n", quest.Region, r.Request.URL, err)
	})

	fmt.Printf("Total quests to visit: %d\n", len(quests))
	for i, quest := range quests {
		fmt.Printf("Queuing quest %d: %s - %s\n", i+1, quest.Region, quest.Name)
		ctx := colly.NewContext()
		ctx.Put("quest", quest)
		err := c.Request("GET", quest.Link, nil, ctx, nil)
		if err != nil {
			fmt.Printf("Error queuing request for %s - %s: %s\n", quest.Region, quest.Name, err)
		}
	}

	c.Wait()
	fmt.Println("All quests have been processed.")
}

// Add this new function outside of visitQuests, at the same level as other functions
func findRewardsInHousyuTable(e *colly.HTMLElement) bool {
	rewardsFound := false
	e.DOM.Find("#article-body > div.genshin_housyu > table > tbody > tr").Each(func(_ int, row *goquery.Selection) {
		rewardName := strings.TrimSpace(row.Find("th").Text())
		rewardValue := strings.TrimSpace(row.Find("td").Text())
		
		if rewardName != "" && rewardValue != "" {
			if rewardName == "Items" {
				items := strings.Split(rewardValue, "\n")
				for _, item := range items {
					item = strings.TrimSpace(item)
					if item != "" {
						fmt.Printf("- %s\n", item)
						rewardsFound = true
					}
				}
			} else {
				fmt.Printf("- %s x %s\n", rewardName, rewardValue)
				rewardsFound = true
			}
		}
	})
	return rewardsFound
}


// Add this new function outside of visitQuests, at the same level as other functions
func findRewardsInSpecificTable(e *colly.HTMLElement) bool {
    rewardsFound := false
    e.DOM.Find("#article-body > table:nth-child(34) > tbody > tr").Each(func(_ int, row *goquery.Selection) {
        cells := row.Find("td")
        if cells.Length() >= 2 {
            rewardName := strings.TrimSpace(cells.Eq(0).Text())
            rewardValue := strings.TrimSpace(cells.Eq(1).Text())
            if rewardName != "" && rewardValue != "" {
                fmt.Printf("- %s x %s\n", rewardName, rewardValue)
                rewardsFound = true
            }
        }
    })
    return rewardsFound
}