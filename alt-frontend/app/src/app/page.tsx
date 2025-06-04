import { Flex } from "@chakra-ui/react";
import css from "./page.module.css";

export default function Home() {
  return (
    <div className={css.container} >
      <Flex justifyContent="center" alignItems="center" height="100vh" >
        <h1>Home</h1>
      </Flex>
    </div>
  );
}