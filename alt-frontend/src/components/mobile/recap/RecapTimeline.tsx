"use client";

import { Flex } from "@chakra-ui/react";
import type { RecapGenre } from "@/schema/recap";
import RecapCard from "./RecapCard";

type RecapTimelineProps = {
  genres: RecapGenre[];
};

const RecapTimeline = ({ genres }: RecapTimelineProps) => {
  return (
    <Flex direction="column" gap={4} data-testid="recap-timeline">
      {genres.map((genre) => (
        <RecapCard key={genre.genre} genre={genre} />
      ))}
    </Flex>
  );
};

export default RecapTimeline;
