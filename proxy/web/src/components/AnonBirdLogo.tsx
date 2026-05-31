import { cn } from "@/utils/helpers";
import anonbirdFull from "@/assets/anonbird-full.svg";
import anonbirdMark from "@/assets/anonbird.svg";

type Props = {
  size?: "small" | "default" | "large";
  mobile?: boolean;
};

const sizes = {
  small: {
    desktop: 14,
    mobile: 20,
  },
  default: {
    desktop: 22,
    mobile: 30,
  },
  large: {
    desktop: 24,
    mobile: 40,
  },
};

export const AnonBirdLogo = ({ size = "default", mobile = true }: Props) => {
  return (
    <>
      <img
        src={anonbirdFull}
        height={sizes[size].desktop}
        style={{ height: sizes[size].desktop }}
        alt="AnonBird Logo"
        className={cn(mobile && "hidden md:block", "group-hover:opacity-80 transition-all")}
      />
      {mobile && (
        <img
          src={anonbirdMark}
          width={sizes[size].mobile}
          style={{ width: sizes[size].mobile }}
          alt="AnonBird Logo"
          className={cn(mobile && "md:hidden ml-4")}
        />
      )}
    </>
  );
};
