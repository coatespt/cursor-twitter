# Word Filter Lists

This directory contains comprehensive lists of words that should be filtered out of the anomaly detection system.

## File Structure

```
filter_lists/
├── README.md                    # This file
├── slurs.txt                    # Racial, ethnic, and other slurs
├── profanity.txt                # Vulgar language and profanity
├── spam_words.txt               # Spam and bot indicators
├── noise_words.txt              # Common noise (lol, omg, etc.)
├── abbreviations.txt            # Common abbreviations
├── social_media_noise.txt       # Platform-specific noise
└── custom_filters.txt           # Custom filters for your use case
```

## Managing Large Filter Lists

### **Why Separate Files?**
- **Maintainability**: Easier to update specific categories
- **Performance**: Load only needed categories
- **Collaboration**: Different people can maintain different lists
- **Size**: Some lists (like slurs) can be thousands of entries

### **Sources for Comprehensive Lists**
- **Slurs**: Academic research papers, hate speech datasets
- **Profanity**: Content filtering libraries
- **Spam**: Bot detection research
- **Noise**: Social media analysis studies

### **Recommended Approach**
1. **Start with basic lists** (provided here)
2. **Add comprehensive slur lists** from research sources
3. **Monitor system output** and add new patterns
4. **Regular updates** as new terms emerge

### **File Format**
- One word per line
- `#` for comments
- Case-insensitive (system handles this)
- No special characters needed

### **Loading Strategy**
```go
// Load all filter categories
filters := LoadFilterCategories(config.FilterCategories)

// Or load specific categories
slurs := LoadFilterList("slurs.txt")
profanity := LoadFilterList("profanity.txt")
```

### **Performance Considerations**
- **Bloom filters**: For very large lists (>10k words)
- **Hash sets**: For smaller lists
- **Lazy loading**: Load only enabled categories
- **Caching**: Keep frequently used lists in memory

### **Maintenance**
- **Automated updates**: Scripts to pull from authoritative sources
- **Community contributions**: Allow users to submit new patterns
- **Validation**: Regular checks for false positives
- **Versioning**: Track changes to filter lists

## Getting Started

1. **Copy the basic lists** provided here
2. **Add comprehensive slur lists** from research sources
3. **Customize for your use case** in `custom_filters.txt`
4. **Test thoroughly** to ensure legitimate content isn't filtered
5. **Monitor and update** regularly

## Note on Slur Lists

The slur lists can be very large and sensitive. Consider:
- Using established academic datasets
- Community-maintained lists
- Regular updates as language evolves
- Careful testing to avoid over-filtering 