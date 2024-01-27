# GoFishCustomSearcherCaseStudy
Case study for customizing the search module of the GoFish engine.

For demonstration purposes, the custom search implementaion simply copies the default search implementation and removes the following optimizations:
1. Transpostion Table
2. Null Move Pruning
3. Singular Move Extension
4. Quiscensce Search
5. Internal Iterative Deepining

This will naturally make this custom engine less powerful than the default implementation of GoFish.
